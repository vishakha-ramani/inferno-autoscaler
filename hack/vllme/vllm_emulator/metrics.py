import prometheus_client
from typing import List

class Metrics:

    def __init__(self, labelnames: List[str]):
        self.gauge_scheduler_running = prometheus_client.Gauge(
            name="vllm:num_requests_running",
            documentation="Number of requests currently running on GPU.",
            labelnames=labelnames)
        self.gauge_scheduler_waiting = prometheus_client.Gauge(
            name="vllm:num_requests_waiting",
            documentation="Number of requests waiting to be processed.",
            labelnames=labelnames)
        #   KV Cache Usage in %
        self.gauge_gpu_cache_usage = prometheus_client.Gauge(
            name="vllm:gpu_cache_usage_perc",
            documentation="GPU KV-cache usage. 1 means 100 percent usage.",
            labelnames=labelnames)
        
        request_latency_buckets = [
            0.3, 0.5, 0.8, 1.0, 1.5, 2.0, 2.5, 5.0, 10.0, 15.0, 20.0, 30.0,
            40.0, 50.0, 60.0, 120.0, 240.0, 480.0, 960.0, 1920.0, 7680.0
        ]

        """
        Counter for request arrivals
        """
        self.counter_request_arrival_total = prometheus_client.Counter(
            name="vllm:request_arrival", # prometheus adds a _total suffix at the end
            documentation="Total number of requests received (arrivals).",
            labelnames=labelnames)
        
        """
        Counter for request successes (departures)
        """
        self.counter_request_success_total = prometheus_client.Counter(
            name="vllm:request_success",
            documentation="Total number of requests successfully completed (departures).",
            labelnames=labelnames)
        
        """
        Counter for total number of tokens generated (input + output)
        """
        self.counter_tokens_total = prometheus_client.Counter(
            name="vllm:tokens", # prometheus adds a _total suffix at the end
            documentation="Total number of tokens generated.",
            labelnames=labelnames)
        
        """
        Histogram for Inter-Token Latency (ITL)
        """
        self.histogram_time_per_output_token = prometheus_client.Histogram(
            name="vllm:time_per_output_token_seconds",
            documentation="Histogram of inter-token latency in seconds.",
            labelnames=labelnames,
            buckets=[
                0.01, 0.025, 0.05, 0.075, 0.1, 0.15, 0.2, 0.3, 0.4, 0.5, 0.75,
                1.0, 2.5
        ])
        
        """
        Histogram for time spent in waiting
        """
        self.histogram_queue_time_request = prometheus_client.Histogram(
            name="vllm:request_queue_time_seconds",
            documentation="Histogram of time spent in WAITING phase for request.",
            labelnames=labelnames,
            buckets=request_latency_buckets)  
        
        """
        Histogram for number of generation tokens
        """
        self.histogram_num_generation_tokens_request = prometheus_client.Histogram(
            name="vllm:request_generation_tokens",
            documentation="Number of generation tokens processed.",
            labelnames=labelnames,
            buckets=[
                1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000
            ])
