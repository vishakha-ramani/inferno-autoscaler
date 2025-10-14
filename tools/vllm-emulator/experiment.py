import random

import matplotlib.pyplot as plt
import numpy
from vllm_model import *

open('logs/run.log', 'w').close() #clear logs


SORTED_FLAG = False       # Sort the request according to their input token length while adding to vLLM
MUTE_PRINT  = False
NUM_RUNS    = 1

VARIATIONS = ['normal' ] #['wq_sorted_by_input_length', 'normal' ]

for var in VARIATIONS:
    print(f"------------- Variation : {var} ---------------")
    memory_available = []
    ttft_times = []
    total_times = []
    processing_times = []
    execution_times = []
    avg_memory_available = []

    for it in range(NUM_RUNS):
        memory_available_perrun = []
        
        clock = Clock(start_time = 0, step_time = DECODE_TIME)
        model = Model(model_id = 1, model_size = 25000, kvcache_per_token = KVC_PER_TOKEN)
        gpu   = Device(device_id = 1, net_memory = M, useable_ratio = 0.8)

        if var == 'normal':
            vllmi = vLLM( device=gpu, clock=clock, model=model, max_batch_size=100)
        elif var == 'wq_sorted_by_input_length':
            vllmi = vLLM_varitaion_sorted_wq( device=gpu, clock=clock, model=model, max_batch_size=100)

        ##### Generate aritificial requests ######

        RequestQueue = []     # TODO: later requests will arrive and be added to the queue
        NUM_REQ = 100

        for rid in range(NUM_REQ):
            input_len = 10 + random.randint(0, 100)
            output_len = input_len + random.randint(5, 100)
            reqi = RequestElement(req_id=rid, input_token_length=input_len, output_token_length=output_len)
            RequestQueue.append(reqi)

        if SORTED_FLAG:
            RequestQueue.sort(key=lambda x: x.InputTokenLength)

        
        ######## Add requests to vLLM ###########
        while len(RequestQueue) > 0:
            reqj = RequestQueue.pop(0)
            vllmi.add_new_request(reqj)

        if not MUTE_PRINT:
            print("Req in run queue:")
            for r in vllmi.running_queue:
                print(r.ReqId, " - ", r.token_len)

            print("Req in wait queue:")
            for r in vllmi.waiting_queue:
                print(r.ReqId, " - ", r.token_len)

        ######### Start execution ################
            
        iter_no=0
        while len(vllmi.finished_queue) < NUM_REQ:
            iter_no += 1
            if not MUTE_PRINT:
                vllmi.print_spec()
            vllmi.one_iteration()
            memory_available_perrun.append(vllmi.Device.get_available_memory())
            memory_available.append(vllmi.Device.get_available_memory())
            #time.sleep(0.5)

        execution_times.append(vllmi.Clock.get_curr_time()/1000)
        avg_memory_available.append(numpy.average(memory_available_perrun))

        for reqi in vllmi.finished_queue:
            total_time = reqi.completion_time - reqi.arrival_time
            processing_time = total_time - reqi.ttft_met_time
            ttft_times.append(reqi.ttft_met_time/1000)
            total_times.append(total_time/1000)
            processing_times.append(processing_time/1000)

        #     print(f"Request {reqi.ReqId} : TTFT = {reqi.ttft_met_time} : Time to complete = {total_time} : Processing time = {total_time - reqi.ttft_met_time} : Seq len = {reqi.token_len} ")
        
    print("Average: ", numpy.average(execution_times))
    print("STD: ", numpy.std(execution_times))

    print("Average mem: ", numpy.average(avg_memory_available))
    print("STD: mem", numpy.std(avg_memory_available))



    ############## Plot memory histogram
    plt.hist([100*m/(80000-25000) for m in memory_available[:-100]], bins=list(range(0,50,5)), alpha = 0.7, label=var)   ## Normalized by (GPU mem - model mem)
    plt.xlabel("Percentage of memory available for KVCache (i.e GPU Memory - Model Size)")
    plt.ylabel("Number of iterations")
    plt.title("Histogram of free memory available per iteration")


    ############### Plot TTFT histogram
    # plt.hist(ttft_times[:], bins=20, alpha = 0.7, label=var)   ## Normalized by (GPU mem - model mem)
    # plt.xlabel("TTFT times in sec")
    # plt.ylabel("Number of requests")
    # plt.title("Histogram of requests by TTFT")


    ############### Plot processing time histogram
    # plt.hist(processing_times[:], bins=list(range(0,60,5)), alpha = 0.7, label=var)   ## Normalized by (GPU mem - model mem)
    # plt.xlabel("Processing times in sec")
    # plt.ylabel("Number of requests")
    # plt.title("Histogram of requests by processing times")

    #plt.show()

plt.legend(loc="upper right")
plt.show()


