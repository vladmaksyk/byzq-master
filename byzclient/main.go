package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/relab/gorums/cmd/byzq-master/byzq"

	"google.golang.org/grpc"
)

func main() {
	var (
		port = flag.Int("port", 8080, "port where local server is listening")
		//saddrs = flag.String("addrs", ":8081,:8082,:8083,:8084,:8085,:8086,:8087", "server addresses separated by ','")
		saddrs = flag.String("addrs", ":8081,:8082,:8083,:8084", "server addresses separated by ','")
		f      = flag.Int("f", 1, "fault tolerance, supported values f=1,2,3 (this is ignored if addrs is provided)")
		//noauth   = flag.Bool("noauth", true, "don't use authenticated channels")
		generate = flag.Bool("generate", false, "generate public/private key-pair and save to file provided by -key")
		//writer   = flag.Bool("writer", false, "set this client to be writer only (default is reader only)")
		keyFile = flag.String("key", "priv-key.pem", "private key file to be used for signatures")
		numReq  = flag.Int("numReq", 1000, "Number of requests")
	)
	//Address = []string{"pitter1:12100","pitter2:12100", "pitter3:12100"}

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *generate {
		// Generate key file and exit.
		err := byzq.GenerateKeyfile(*keyFile)
		if err != nil {
			dief("error generating public/private key-pair: %v", err)
		}
		os.Exit(0)
	}

	if *saddrs == "" {
		fmt.Println("inside")
		// Use local addresses only..
		if *f > 3 || *f < 1 {
			dief("only f=1,2,3 is allowed")
		}
		n := 3**f + 1
		var buf bytes.Buffer
		for i := 0; i < n; i++ {
			buf.WriteString(":")
			buf.WriteString(strconv.Itoa(*port + i))
			buf.WriteString(",")
		}
		b := buf.String()
		*saddrs = b[:len(b)-1]
	}

	addrs := strings.Split(*saddrs, ",")
	fmt.Println("Default servers ->", addrs)

	if len(addrs) == 0 {
		dief("no server addresses provided")
	}
	log.Printf("#addrs: %d (%v)", len(addrs), *saddrs)

	// Set Dial options
	grpcOpts := []grpc.DialOption{grpc.WithBlock()}
	grpcOpts = append(grpcOpts, grpc.WithInsecure())
	dialOpts := byzq.WithGrpcDialOptions(grpcOpts...)

	// Create manager
	mgr, err := byzq.NewManager(addrs, dialOpts, byzq.WithTracing(), byzq.WithDialTimeout(30*time.Second))
	defer mgr.Close()
	if err != nil {
		dief("error creating manager: %v", err)
	}
	fmt.Println("Managed Connections and Created a manager->", mgr)
	ids := mgr.NodeIDs()
	fmt.Println("mgr.NodeIDs() ->", ids)

	// Reading key file
	fmt.Println("Reading key file...")
	key, err := byzq.ReadKeyfile(*keyFile)
	if err != nil {
		dief("error reading keyfile: %v", err)
	}

	// Creating Authorization data
	fmt.Println("Creating NewAuthDataQ...")
	qspec, err := byzq.NewAuthDataQ(len(ids), key, &key.PublicKey)
	if err != nil {
		dief("error creating quorum specification: %v", err)
	}

	// Creating ne configuration
	fmt.Println("Creating NewConfiguration...")
	conf, err := mgr.NewConfiguration(ids, qspec)
	if err != nil {
		dief("error creating config: %v", err)
	}

	fmt.Println("=======Clients configuration ->", conf.NodeIDs())

	//Creating new storage state
	fmt.Println("Creating new storageState...")
	storageState := &byzq.Content{
		Key:       "ClienttoServers",
		Value:     "Write",
		Timestamp: -1,
		Echowrite: true,
		Port:      int64(*port),
	}
	fmt.Println("StorageState created ->", storageState)

	start := time.Now()
	// Writing to servers
	//operations := 100
	for i := 0; i < *numReq; i++ {
		//fmt.Println("==========Client Writing to servers...==============")
		storageState.Value = strconv.Itoa(rand.Intn(1 << 8))
		storageState.Timestamp++
		signedState, err := qspec.Sign(storageState)
		if err != nil {
			dief("failed to sign message: %v", err)
		}
		ack, err := conf.Write(context.Background(), signedState)
		if err != nil {
			dief("error writing: %v", err)
		} else {
			fmt.Println("Got acknowlegement that the quorum of servers replyed ", ack)
		}
	}
	elapsed := time.Since(start)
	fmt.Println("Time for ", *numReq, " operations ->", elapsed)
	//time.Sleep(5 * time.Second)

}

func dief(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format, a...)
	fmt.Fprint(os.Stderr, "\n")
	flag.Usage()
	os.Exit(2)
}

/////////////////////////////////////////////////////////////////////////////////////////////////////
// Secure Dial with grpc
// var secDialOption grpc.DialOption
// if *noauth {
// 	fmt.Println("Insecure Dial options")
// 	secDialOption = grpc.WithInsecure()

// 	grpcOpts := []grpc.DialOption{grpc.WithBlock()}
// 	grpcOpts = append(grpcOpts, grpc.WithInsecure())
// 	dialOpts := byzq.WithGrpcDialOptions(grpcOpts...)
// } else {
// 	fmt.Println("Authorization...")
// 	clientCreds, err := credentials.NewClientTLSFromFile("cert/server.crt", "127.0.0.1")
// 	if err != nil {
// 		dief("error creating credentials: %v", err)
// 	}
// 	secDialOption = grpc.WithTransportCredentials(clientCreds)
// 	fmt.Println("Authorization succesefull")
// }
/////////////////////////////////////////////////////////////////////////////////////////////////////
