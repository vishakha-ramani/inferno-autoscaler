import time
import random
import threading
from openai import OpenAI

# Configuration parameters
API_KEY = "fake-api-key"
MODEL = "gpt-1337-turbo-pro-max"
RATE = 60  # default rate (requests per minute)
MEAN_INTERVAL = 60 / RATE  # mean inter-arrival time in seconds
CONTENT_LENGTH = 150  # default content length

# Predefined URL options
URL_OPTIONS = {
    1: "http://localhost:30000/v1",
    2: "http://localhost:30010/v1",
    3: "http://localhost:8000/v1"
}

# Function to simulate a single client sending a request
def send_request():
    client = OpenAI(api_key=API_KEY, base_url=BASE_URL)
    content = "x" * CONTENT_LENGTH

    try:
        chat_completion = client.chat.completions.create(
            messages=[
                {"role": "user", "content": content}
            ],
            model=MODEL,
        )
        # Uncomment below to print API response (can be noisy)
        # print(chat_completion.choices[0].message.content)
    except Exception as e:
        print(f"Request failed: {e}")

# Function to simulate Poisson process and generate requests
def poisson_request_generator():
    client_threads = []
    request_count = 0
    start_time = time.time()

    try:
        while True:
            inter_arrival_time = random.expovariate(1 / MEAN_INTERVAL)
            time.sleep(inter_arrival_time)
            
            thread = threading.Thread(target=send_request)
            thread.start()
            client_threads.append(thread)

            request_count += 1

            if time.time() - start_time >= 60:
                print(f"Total requests sent in the last minute: {request_count}")
                request_count = 0
                start_time = time.time()

            client_threads = [t for t in client_threads if t.is_alive()]
    except KeyboardInterrupt:
        print("\nLoad generator stopped by user.")


def main():
    global BASE_URL, MODEL, RATE, MEAN_INTERVAL, CONTENT_LENGTH

    print("Select the server base URL:")
    for option, url in URL_OPTIONS.items():
        print(f"{option}: {url}")

    while True:
        try:
            choice = int(input("Enter the option number (1/2): ").strip())
            if choice in URL_OPTIONS:
                BASE_URL = URL_OPTIONS[choice]
                break
            else:
                print("Invalid option. Please choose 1 or 2.")
        except ValueError:
            print("Invalid input. Please enter a valid number (1 or 2).")

    MODEL = input("Enter the model name (e.g., gpt-1337-turbo-pro-max): ").strip()
    RATE = int(input("Enter the rate (requests per minute): ").strip())
    MEAN_INTERVAL = 60 / RATE
    CONTENT_LENGTH = int(input("Enter the content length (e.g., 100-200): ").strip())

    print("Starting load generator...")
    print(f"Server Address: {BASE_URL}")
    print(f"Request Rate = {RATE}")
    print(f"Model: {MODEL}")
    print(f"Content Length: {CONTENT_LENGTH}")
    poisson_request_generator()

if __name__ == "__main__":
    main()
