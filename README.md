To simulate the execution follow these simple insructions:
----------------------------------------------------------

* Run four servers as four different processes:

    ```console
    cd byzserver 
    go run main.go -port=8081 -key keys/server
	go run main.go -port=8082 -key keys/server
	go run main.go -port=8083 -key keys/server
	go run main.go -port=8084 -key keys/server
    ```

* Then run a writer client, and specify the number of requests (default 1000)

    ```console
    cd byzclient
    go run main.go -port=8080 -numReq=100
    ```

## Compile .proto file

* If you make any changes to the proto file, compile it with this command.

    ```console
    cd byzq
    protoc byzq.proto --gorums_out=plugins=grpc+gorums:.
    ```
