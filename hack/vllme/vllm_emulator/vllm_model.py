'''
D = number of devices
M = Memory in the Devices (assumed equal), in MB - TODO: Change it for MIG later

KVC_PER_TOKEN = net KV cache size per token, in MB - assumed same for all requests
DECODE_TIME   = Time per decode step in ms - assumed independent of batch size of requests runnning - TODO: Verify

NOTE: Prefill has not been modelled. Prefill and decode are assumed to take same time.

'''
import asyncio
####----------------------------------- Logging Setup --------------------------------------
import logging
import math
import os
import random
from pathlib import Path

from metrics import Metrics

logger = logging.getLogger(__name__)
#FORMAT = "[%(filename)s:%(lineno)s - %(funcName)20s() - %(levelname)-5.5s] %(message)s"
FORMAT = "[%(levelname)-5.5s] %(message)s"
PRINT_WHERE = "FILE_ONLY"  ## [FILE_ONLY, TERMINAL]

Path("logs").mkdir(exist_ok=True)

if PRINT_WHERE == "TERMINAL":
##### To Print to both file and terminal
    logging.basicConfig( handlers=[
            logging.FileHandler("logs/run.log", mode='w'),
            logging.StreamHandler()], encoding='utf-8', level=logging.DEBUG, format=FORMAT)
elif PRINT_WHERE == "FILE_ONLY":
    ### To only print to log file
    logging.basicConfig( handlers=[
            logging.FileHandler("logs/run.log", mode='w')], encoding='utf-8', level=logging.DEBUG, format=FORMAT)

###---------------------------------- Global Settings -------------------------------------

MAX_SEQ_LEN   = 2048      # TODO: Currently not used
INF           = float('inf')
MUTE_PRINT    = False

###------------------------------- Classes ----------------------------------------------

class Clock:
    def __init__(self, start_time: float, step_time: float, realtime: bool = True):
        '''
        step_time: duration of one decode iteration in ms
        realtime: if True, sleep for step_time to simulate real-time progression
        '''
        self.start_time = start_time
        self.step_time = step_time
        self.curr_time = start_time
        self.realtime = realtime

    async def time_step(self, no_steps: int = 1):
        if self.realtime:
            await asyncio.sleep(no_steps * self.step_time / 1000)  # step_time is in ms
        self.curr_time += no_steps * self.step_time
        return self.curr_time

    def get_curr_time(self):
        return self.curr_time


class Model():
    def __init__(self, model_name: str, model_size: int = 25000, kvcache_per_token: int = 1, decode_time: int = 50, prefill_time: int = 100):
        self.model_name      = model_name
        self.KVcachePerToken = kvcache_per_token
        self.ModelSize       = model_size          ## size in MB
        self.PrefillTime     = prefill_time        # in ms
        self.DecodeTime      = decode_time         # in ms

    def run_one_iteration(self, request_list=[], any_prefill = False):  # TODO: Add prefill time consideration
        return self.DecodeTime                     # time duration of one forward run as a function of requests scheduled


class Device:
    def __init__(self, device_id, net_memory, metrics, model_name, useable_ratio = 0.8):
        self.DevId         = device_id
        self.Memory        = net_memory
        self.useable_ratio = useable_ratio
        self.mem_usage     = 0                # used memory at any give time
        self.running_req   = []               # set of running requests at given time

        self.loaded_model = None              # Object of Model() # Assumes only one model loaded at a time
        self.model_name = model_name
        self.metrics = metrics

    def get_available_memory(self):
        '''How much free memory does the device have'''
        available_memory = (self.Memory*self.useable_ratio - self.mem_usage)
        assert available_memory >= 0
        return available_memory

    def check_memory_availability(self, mem_request):   # If True, you can ask for mem_request
        '''Check if mem_request can be fulfilled'''
        if self.get_available_memory() >=  mem_request:
            return True
        else:
            return False

    def use_memory(self, mem_request):
        '''Use specified amount of memory. Raise error if enough memory isn't available'''
        logger.debug(f"Device {self.DevId} memory request of {mem_request}. Available memory on device = {self.get_available_memory()}")
        assert self.check_memory_availability(mem_request)
        self.mem_usage = self.mem_usage + mem_request
        self._update_metrics()
        return True

    def release_memory(self, mem_release):
        self.mem_usage = self.mem_usage - mem_release
        assert self.mem_usage >=0
        self._update_metrics()
        return True

    def load_model(self, model : Model):
        '''
        Reserve memory for model weights
        model: object of Model()
        '''
        if self.loaded_model is None:
            assert self.check_memory_availability(model.ModelSize)
            self.mem_usage = self.mem_usage + model.ModelSize
            self.loaded_model = model
            self._update_metrics()
            return self.get_available_memory()
        else:
            raise Exception("A Model already loaded. Not allowed to load another model")

    def remove_model(self, model : Model):
        if self.loaded_model is not None:
            self.mem_usage = self.mem_usage - model.ModelSize
            assert self.mem_usage >=0
            self.loaded_model = None
            self._update_metrics()
            return True
        else:
            return False

    def _update_metrics(self):
        usagePercent = self.mem_usage / self.Memory * 100.0
        self.metrics.gauge_gpu_cache_usage.labels(model_name=self.model_name).set(usagePercent)



class Request:
    def __init__(self, req_id, input_token_length, output_token_length = MAX_SEQ_LEN):
        self.ReqId             = req_id
        self.InputTokenLength  = input_token_length
        self.OutputTokenLength = output_token_length  # in real, this is not known. For modelling, we use this to predefine request size

        self.token_len         = input_token_length  # Current token length


class RequestElement(Request):
    '''
    Request when added to the serving system (it'll be in one queue or the other - eg: vllm running q, or waiting q or global q)
    '''
    def __init__(self, req_id, input_token_length, output_token_length = MAX_SEQ_LEN, arrival_time = None, priority_class = 0,  ttft = INF, slo = INF):
        Request.__init__(self, req_id, input_token_length, output_token_length)

        self.TTFT  = ttft                      # Expected TTFT to be met
        self.SLO   = slo                       # NOTE: not used
        self.PriorityClass = priority_class    # higher class is higher priority

        self.arrival_time = arrival_time       # Time when the request arrived at the serving system
        self.completion_time = None            # Time at which last token is generated
        self.ttft_met_time = INF               # TTFT from arrival time
        self.token_times = []                  # Times at which each token of the sequence was generated

        self.stage       = 'not_run_yet'       # 'not_run_yet', 'waiting', 'decode' or 'prefill' or 'finished' # NOTE: not using prefill currently

        self.event = None                      # completion event to be set when the request terminates
        self.entered_waiting_queue_time = -(math.inf)     # time that the request entered in waiting queue


    def add_new_tokens(self, num_tokens, curr_time):
        '''
        As LLM generates new token, add it to the request element.
        Returns actual number of tokens added
        '''
        assert self.stage != 'finished'
        assert num_tokens > 0
        if self.stage == 'not_run_yet':        ## TODO: Add a prefill stage
            self.stage = 'decode'
            self.ttft_met_time = curr_time - self.arrival_time  ## record TTFT time
        new_token_length = min( self.token_len + num_tokens, self.OutputTokenLength )
        self.token_times.extend([curr_time for _ in range(new_token_length - self.token_len)])
        self.token_len = new_token_length
        if self._is_request_complete():
            self.stage = 'finished'
            self.completion_time = curr_time
        return self.token_len                 ## NOTE: Currently not checked since we only generate one token at a time. Will change when adding speculative decoding

    def _is_request_complete(self):
        '''
        Note: For modelling, we assume that the request lengths are preset
        '''
        if self.token_len < self.OutputTokenLength:
            return False
        else:
            return True

##---------------------------------------------------------------- vLLM Model ------------------------------------------------------------------------

## Source: https://realpython.com/inherit-python-list/
class CustomList(list):
    def __init__(self, iterable):
        super().__init__(item for item in iterable)

    def __setitem__(self, index, item):
        super().__setitem__(index, item)

    def insert(self, index, item):
        super().insert(index, item)

    def append(self, item):
        super().append(item)


class SortedByTokenLenList(list):
    '''
    Requests in this list will always be sorted by their current token lengths
    '''
    def __init__(self, iterable):
        super().__init__(item for item in iterable)

    def __setitem__(self, index, item):
        super().__setitem__(index, item)

    def insert(self, index, item):
        super().insert(index, item)
        super().sort(key=lambda x: x.token_len)

    def append(self, item):
        super().append(item)
        super().sort(key=lambda x: x.token_len)


#new_list = sorted(orig_list, key=lambda x: x.count, reverse=True)
#orig_list.sort(key=lambda x: x.count, reverse=True)

def get_queue_of_type(queue_type = "default"):
    ## TODO: Add new types like priority queues etc
    if queue_type == "default":
        return CustomList([])
    if queue_type == "sorted_by_token_len":
        return SortedByTokenLenList([])
    return CustomList([])


class vLLM():
    '''
    Class models then functioning of vLLM. Child classes can be created to emulate
    different run queue, waiting queue policies etc.
    '''
    def __init__(self, device : Device, clock : Clock, model : Model, metrics : Metrics, max_batch_size: int):
        self.Device = device
        self.Clock = clock
        self.Model = model

        self.running_queue  = get_queue_of_type("default")
        self.waiting_queue  = get_queue_of_type("default")
        self.finished_queue = get_queue_of_type("default")

        self.stop    = False

        self.max_kvcache_mem = self.Device.load_model(self.Model)
        self.metrics = metrics

        self.max_batch_size = max_batch_size

    async def run_print_spec(self):
        while True:
          self.print_spec()
          await asyncio.sleep(2)

    def print_spec(self):
        print("\n--------------------------******** Current Specs ************--------------------------")
        print("Time in sec:                  ", self.Clock.get_curr_time()/1000)
        print("Device memory available:      ", self.Device.get_available_memory())
        print("Length of vllm running queue: ", len(self.running_queue))
        print("Length of vllm waiting queue: ", len(self.waiting_queue))
        print("Number of Finished requests:  ", len(self.finished_queue))


    def _mem_requirement(self, request :RequestElement):
        '''Memory requirement given a request on this model'''
        return (request.token_len) * self.Model.KVcachePerToken

    def _can_request_run(self, request : RequestElement):
        '''
        Check if max batch size is satisfied
        '''
        if len(self.running_queue) + 1 > self.max_batch_size:
            return False
                
        '''
        Checks if this device has enough memory to run this request for atlest one token.
        Has to account for all current requests on the device
        '''
        print(f"Can run: {request.ReqId}, {request.token_len}")
        print(f"Req memory {self._mem_requirement(request) + self.Model.KVcachePerToken * (len(self.running_queue) + 1)}")
        print(f"Available: {self.Device.get_available_memory()} ")
        assert self._mem_requirement(request) < self.max_kvcache_mem , f"{request.InputTokenLength}, but {self.max_kvcache_mem}"   ### Request should be smaller than what can be fit in the GPU
        if self.Device.check_memory_availability( self._mem_requirement(request) + self.Model.KVcachePerToken * (len(self.running_queue) + 1) ): # should be able to generate atleast one more token than input (along with 1 token for all req in running quque)
            return True
        else:
            return False

    def _add_to_running_queue(self, request : RequestElement):
        '''
        Add a new request to vLLM running queue.
        TODO: running_queue's append can be modified to do more elaborate append eg: priority queue
        '''
        assert self._can_request_run(request)
        self.Device.use_memory(self._mem_requirement(request))
        self.running_queue.append(request)
        self.metrics.gauge_scheduler_running.labels(model_name=self.Model.model_name).inc()

    def _add_to_waiting_queue(self, request : RequestElement):
        '''
        Add a new incoming request to vLLM waiting queue.
        TODO: waiting_queue's append can be modified to do more elaborate append eg: priority queue
        '''
        request.entered_waiting_queue_time = self.Clock.get_curr_time() / 1000  # seconds
        self.waiting_queue.append(request)
        self.metrics.gauge_scheduler_waiting.labels(model_name=self.Model.model_name).inc()

    def _remove_from_running_queue(self, request : RequestElement):
        '''
        Remove a given request from running queue because 1. it finished or 2. it's evicted
        Which request is to be evicted, that policy is separate - see where this fucntion is used.
        '''
        self.Device.release_memory(self._mem_requirement(request))
        self.running_queue.remove(request)
        self.metrics.gauge_scheduler_running.labels(model_name=self.Model.model_name).dec()

    def _remove_from_waiting_queue(self, request : RequestElement):
        '''
        Same as _remove_from_running_queue() for waiting queue
        '''
        self.waiting_queue.remove(request)
        self.metrics.gauge_scheduler_waiting.labels(model_name=self.Model.model_name).dec()


    ### TODO: Policy for eviction between running and waiting queue of VLLM
    def _move_from_running_to_waiting_queue(self): # NOTE: currently just moves tail of running queue to the head of running queue
        '''
        Remove from running queue and insert at the head of waiting queue
        '''
        tail_req = self.running_queue[-1]
        self._remove_from_running_queue(tail_req)
        self.waiting_queue.insert(0,tail_req)         ## Note insert may or maynot be to position 0, depends on th what kind of queue waiting_queu is (eg: priority queue)
        logger.info(f" <-- Evicted request ** {tail_req.ReqId} ** with token length of ** {tail_req.token_len} **. Memory Available: {self.Device.get_available_memory()} ")

    ### TODO: Policy for adding from waiting queue to running queue of vLLM
    def _move_from_waiting_to_running_queue(self):  #TODO: Currently moves head of waiting queue to tail of runnign queqe
        head_req = self.waiting_queue.pop(0)

        now = self.Clock.get_curr_time() / 1000  # seconds
        wait_time = now - getattr(head_req, 'entered_waiting_queue_time', now)
        if math.isfinite(wait_time) and wait_time >= 0:
            self.metrics.histogram_queue_time_request.labels(model_name=self.Model.model_name).observe(wait_time)

        logger.info(f" --> Adding request ** {head_req.ReqId} ** to running queue with token length of ** {head_req.token_len} **. Memory Available: {self.Device.get_available_memory()} ")
        self._add_to_running_queue(head_req)

    def _add_to_vllm_queue(self, request: RequestElement):
        '''
        Adds to waiting queue. But if waiting queue is empty adds to running queue
        '''
        if len(self.waiting_queue) == 0:
            if self._can_request_run(request):
                self._add_to_running_queue(request)
            else:
                self._add_to_waiting_queue(request)
        else:
            self._add_to_waiting_queue(request)

    async def add_new_request_wait(self, request: RequestElement):
        completionEvent = asyncio.Event()
        request.event = completionEvent
        self.add_new_request(request)
        await completionEvent.wait()
        # update metrics: total number of tokens generated
        self.metrics.counter_tokens_total.labels(model_name=self.Model.model_name).inc(request.token_len)
        self.metrics.histogram_num_generation_tokens_request.labels(model_name=self.Model.model_name).observe(request.token_len)
        self.remove_finished_request(request)

    def add_new_request(self, request: RequestElement):    #TODO: We assume only 1 vLLM instance. Eventually the Request will be added as Request element at a global queue and then fed to vLLM queue
        '''
        Add a new request to vLLM queue. Either it runs right away or waits at the end of waiting queue
        '''
        request.arrival_time = self.Clock.get_curr_time()
        # update metrics: total number of request arrivals
        self.metrics.counter_request_arrival_total.labels(model_name=self.Model.model_name).inc()
        self._add_to_vllm_queue(request)

    def _evict_requests_for_next_iteration(self):
        ## Remove requests and put to waiting quque if can't run
        mem_required_next_run = 0
        eviction_needed       = False
        for i in range(len(self.running_queue)):
            mem_required_next_run +=  self.Model.KVcachePerToken * 1  # TODO: Assumes ony one token per request
            logger.debug(f"Memory required for {i} requests in next iteration = {mem_required_next_run}")
            if not self.Device.check_memory_availability(mem_required_next_run):
                self._move_from_running_to_waiting_queue()
                mem_required_next_run -=  self.Model.KVcachePerToken * 1
                eviction_needed = True
        return eviction_needed

    def _add_requests_for_next_iteration(self):
        while True:
            if len(self.waiting_queue) > 0:
                if self._can_request_run(self.waiting_queue[0]):
                    self._move_from_waiting_to_running_queue()
                else:
                    break
            else:
                break

    def remove_finished_request(self, request: RequestElement):
        if request in self.finished_queue:
            self.finished_queue.remove(request)
            return request
        else:
            return False

    async def one_iteration(self):
        # TODO: In speculative decoding, there might be more than one token for some requets
        ## Step all running requests by one
        #logger.critical("------------- vLLM iteration starting: generating next set of tokens -----------")
        curr_time = await self.Clock.time_step()
        
        if len(self.running_queue) > 0: # Only observe if there are requests running to generate tokens
            self.metrics.histogram_time_per_output_token.labels(model_name=self.Model.model_name).observe(self.Model.DecodeTime / 1000.0)

        for req in self.running_queue:
            no_new_tokens = 1                                         # TODO: Assumes only one token per request
            mem_required = self.Model.KVcachePerToken * no_new_tokens
            self.Device.use_memory(mem_required)
            req.add_new_tokens(no_new_tokens, curr_time)
            if req.stage == 'finished':
                self.finished_queue.append(req)
                self._remove_from_running_queue(req)
                if req.event != None:
                    req.event.set() # notify waiters
                self.metrics.counter_request_success_total.labels(model_name=self.Model.model_name).inc() # increment the counter for finished requests
                logger.info(f"~*~ Finished request {req.ReqId}. Output token length {req.token_len}")

        #logger.critical("******** Making eviction and admitting decisions for next iteration ******")
        if not self._evict_requests_for_next_iteration():   ## TODO: In iteration we evict requests we dont add any request, since evicted request will be at head of waiting quque.
            self._add_requests_for_next_iteration()

    async def run(self):
        if not MUTE_PRINT:
            print("Starting spec reporter")
            asyncio.create_task(self.run_print_spec())

        iter_no = 0
        while not self.stop:
            iter_no += 1
            logger.debug(f"---------- Iteration numer {iter_no} -------------")
            await self.one_iteration()

class vLLM_varitaion_sorted_wq(vLLM):
    def __init__(self, device : Device, clock : Clock, model : Model, max_batch_size: int):
        super().__init__(device, clock, model, max_batch_size=max_batch_size)
        self.waiting_queue  = get_queue_of_type("sorted_by_token_len")


# TODO: Global queue that holds all entering requests and feeds them into approprite vLLM instance
########################################################################################################################################################################

class Load():
    def __init__(self, avg_generated_len, distribution):
        self.Distribution = distribution
        self.AvgLen       = avg_generated_len

    def _get_generated_len(self):
        match self.Distribution:
            case "uniform":
                return random.randint(0, 2*self.AvgLen)
            case "uniform-narrow":
                return random.randint(self.AvgLen/2, 3*self.AvgLen/2)
            case "deterministic":
                return self.AvgLen
            case _:
                return self.AvgLen

    def get_output_len(self, input_len):
        return input_len + self._get_generated_len()
