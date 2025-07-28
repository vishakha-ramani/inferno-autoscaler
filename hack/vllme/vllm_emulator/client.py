## https://towardsdatascience.com/how-to-build-an-openai-compatible-api-87c8edea2f06
import os

from openai import OpenAI

# init client and connect to localhost server
host_name: str = os.getenv("HOST_NAME", "localhost")
host_port: str = os.getenv("HOST_PORT", "8000")
host_url = "http://" + host_name + ":" + host_port
client = OpenAI(
    api_key="fake-api-key",
    base_url=host_url + "/v1"
)

# call API
chat_completion = client.chat.completions.create(
    messages=[
        {
            "role": "user",
            "content": "Say this is a test",
        }
    ],
    model="gpt-1337-turbo-pro-max",
)

# # print the top "choice"
# print(chat_completion.choices[0].message.content)
