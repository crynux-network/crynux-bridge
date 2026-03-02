# Crynux Bridge

Crynux Bridge is middleware that connects traditional applications to the Crynux Network. It simplifies using the Crynux Network for applications by handling all complex interactions with the Crynux Network. The application only needs to interact with the Crynux Bridge by sending task parameters and waiting for the result images or texts.

More specifically, the Crynux Bridge:

* Manages the application wallet, signs the underlying transactions and API requests.

* Interacts with the blockchain and Relay to execute the entire task workflow.

* Provides simpler APIs to the application to execute tasks using only the task parameters(no blockchain transactions or signatures).

To deploy an instance of the Crynux Bridge and connect your application to it, please follow the tutorial below:

[Deploy Crynux Bridge for Applications](https://docs.crynux.io/application-development/crynux-bridge)

## Build the Docker image from the source code

1. Clone the project

```shell
# git clone https://github.com/crynux-network/crynux-bridge.git
```

2. Go to the root folder of the project

```shell
# cd crynux-bridge
```

3. Build the Docker image

```shell
# docker build -t crynux-bridge:dev -f .\build\crynux_bridge.Dockerfile .
```

4. Start the container

```shell
# docker run -d -p 127.0.0.1:5028:5028 crynux-bridge:dev
```
