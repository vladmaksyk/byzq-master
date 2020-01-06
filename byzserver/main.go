package main

import (
	"context"
	"crypto/ecdsa"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/relab/gorums/cmd/byzq-master/byzq"
)

//n.conn, err = grpc.Dial(n.addr, grpc.WithInsecure())

var (
	buffer *storage
)

type storage struct {
	sync.RWMutex
	//state         map[string]byzq.Value
	serverPort    int
	servers       []string
	privKey       *ecdsa.PrivateKey
	authKey       string
	authorization bool

	configuration *byzq.Configuration
	manager       *byzq.Manager
	qspecs        *byzq.AuthDataQ
	state         *byzq.Content
	signedState   *byzq.Value
}

func main() {
	var (
		//saddrs = flag.String("addrs", ":8081,:8082,:8083,:8084,:8085,:8086,:8087", "server addresses separated by ','")
		saddrs = flag.String("addrs", ":8081,:8082,:8083,:8084", "server addresses separated by ','")
		port   = flag.Int("port", 0000, "port to listen on")
		//f           = flag.Int("f", 1, "fault tolerance")
		noauth      = flag.Bool("noauth", true, "don't use authenticated channels")
		key         = flag.String("key", "", "public/private key file this server")
		privKeyFile = flag.String("privkey", "priv-key.pem", "private key file to be used for signatures")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	//Reading key file
	fmt.Println("Reading key file...")
	privKey, err := byzq.ReadKeyfile(*privKeyFile)
	if err != nil {
		dief("error reading keyfile: %v", err)
	}

	// Splitting addresses
	addrs := strings.Split(*saddrs, ",")
	fmt.Println("Other servers ->", addrs)

	// Creating buffer
	buffer = &storage{serverPort: *port, servers: addrs, privKey: privKey, authKey: *key, authorization: *noauth}

	// Run only one server.
	fmt.Println("Started serving..")
	go buffer.serve()

	// Set Dial options
	grpcOpts := []grpc.DialOption{grpc.WithBlock()}
	grpcOpts = append(grpcOpts, grpc.WithInsecure())
	dialOpts := byzq.WithGrpcDialOptions(grpcOpts...)

	// Create manager
	buffer.manager, err = byzq.NewManager(addrs, dialOpts, byzq.WithTracing(), byzq.WithDialTimeout(30*time.Second))
	defer buffer.manager.Close()
	if err != nil {
		dief("error creating manager: %v", err)
	}
	fmt.Println("Managed Connections and Created a manager->", buffer.manager)
	ids := buffer.manager.NodeIDs()
	fmt.Println("mgr.NodeIDs() ->", ids)

	// Creating new authorization dataQ
	fmt.Println("Creating NewAuthDataQ...")
	buffer.qspecs, err = byzq.NewAuthDataQ(len(ids), privKey, &privKey.PublicKey)
	if err != nil {
		dief("error creating quorum specification: %v", err)
	}

	// Creating Configuration
	fmt.Println("Creating NewConfiguration...")
	buffer.configuration, err = buffer.manager.NewConfiguration(ids, buffer.qspecs)
	if err != nil {
		dief("error creating config: %v", err)
	}
	// Create Storage state for EchoWrite
	fmt.Println("Creating new storageState...")
	buffer.state = &byzq.Content{
		Key:       "ServertoServer",
		Value:     "Echowrite",
		Timestamp: -1,
		Echowrite: false,
		Port:      int64(*port),
	}
	fmt.Println("StorageState created ->", buffer.state)

	//Creating a signed state for EchoWrite
	buffer.state.Value = strconv.Itoa(rand.Intn(1 << 8))
	buffer.state.Timestamp++
	buffer.signedState, err = buffer.qspecs.Sign(buffer.state)
	if err != nil {
		dief("failed to sign message: %v", err)
	}
	fmt.Println("Connected all servers and ready to server client requests")

	time.Sleep(10000 * time.Second)

}

func (b *storage) serve() {
	fmt.Println("Start Listening on port ->", fmt.Sprintf("localhost:%d", b.serverPort))
	l, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", b.serverPort))
	if err != nil {
		log.Fatal(err)
	}

	defer l.Close()

	if b.authKey == "" {
		log.Fatalln("required server keys not provided")
	}

	opts := []grpc.ServerOption{}

	if !b.authorization {
		creds, err := credentials.NewServerTLSFromFile(b.authKey+".crt", b.authKey+".key")
		if err != nil {
			log.Fatalf("failed to load credentials: %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	} else {
		fmt.Println("Running server without authorization")
	}

	// Creating a gRPC server
	fmt.Println("Creating new grpcServer with opts ->", opts)
	grpcServer := grpc.NewServer(opts...)

	byzq.RegisterStorageServer(grpcServer, b)

	// Start serving
	log.Printf("Started serving on port %s ..", l.Addr())
	if err := grpcServer.Serve(l); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}

func (r *storage) Write(ctx context.Context, v *byzq.Value) (*byzq.WriteResponse, error) {
	fmt.Println("Got a write from ", v.C.Port, " and EchoWrite is ->", v.C.Echowrite)

	ack, err := r.configuration.EchoWrite(ctx, r.signedState)
	if err != nil {
		dief("error writing: %v", err)
	} else {
		fmt.Println("Echowriting from : ", r.serverPort, " to other servers was succesefull ->", ack)
	}

	// ack, err = r.configuration.EchoWrite(ctx, r.signedState)
	// if err != nil {
	// 	dief("error writing: %v", err)
	// } else {
	// 	fmt.Println("Echowriting from : ", r.serverPort, " to other servers was succesefull ->", ack)
	// }

	wr := &byzq.WriteResponse{Timestamp: v.C.Timestamp, Port: int64(r.serverPort)}
	return wr, nil
}

func (r *storage) EchoWrite(ctx context.Context, v *byzq.Value) (*byzq.WriteResponse, error) {
	fmt.Println("Got an EchoWrite from ", v.C.Port, " and EchoWrite is ->", v.C.Echowrite)

	// ack, err := r.configuration.EchoEchoWrite(ctx, r.signedState)
	// if err != nil {
	// 	dief("error writing: %v", err)
	// } else {
	// 	fmt.Println("EchoEchoWriting from : ", r.serverPort, " to other servers was succesefull ->", ack)
	// }
	wr := &byzq.WriteResponse{Timestamp: v.C.Timestamp, Port: int64(r.serverPort)}
	return wr, nil
}

func (r *storage) EchoEchoWrite(ctx context.Context, v *byzq.Value) (*byzq.WriteResponse, error) {
	//fmt.Println("Got an EchoEchoWrite from ", v.C.Port, " and EchoEchoWrite is ->", v.C.Echowrite)
	wr := &byzq.WriteResponse{Timestamp: v.C.Timestamp, Port: int64(r.serverPort)}
	return wr, nil
}

func dief(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format, a...)
	fmt.Fprint(os.Stderr, "\n")
	flag.Usage()
	os.Exit(2)
}

func removePort(slice []string, port int) []string {
	var answer []string
	s := strconv.Itoa(port)
	stringport := ":" + s
	for i, value := range slice {
		if value == stringport {
			answer = append(slice[:i], slice[i+1:]...)
		}
	}
	return answer
}
