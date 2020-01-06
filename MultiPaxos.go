package main

import(
	"fmt"
	"net"
	"sync"
	"time"
	"strconv"
	"strings"
	"bytes"
	"os"
	"encoding/gob"
	"encoding/json"
	"dat520.github.io/lab3/detector"
	"dat520.github.io/lab5/bank"
	"dat520.github.io/lab5/multipaxos"
	"math/rand"
	"math"
	"github.com/stats"
	color "github.com/fatih/color"

)

var Address = []string{"localhost:1200","localhost:1201", "localhost:1202","localhost:1203","localhost:1204","localhost:1205"}
//var Address = []string{"172.18.0.0/1", "172.18.0.1/1", "172.18.0.2/1","172.18.0.3/1","172.18.0.4/1"}

var wg = sync.WaitGroup{}

const (
	delta = 6*time.Second
	delta2 = time.Second * 2
)

type Msg struct {
	MsgType   string
	Msg       []byte
	Pid int
	RedirectTo int
}

type IncomingInitialMessage struct {
	Pid int
	Server bool
}

type Process struct {
	ProcessID int
	NumOfProcesses int
	AllProcessIDs []int

	Server bool
	NumOfServers int
	AllServerIDs []int
	ReceivedValue chan bool


	Client bool
	NumOfClients int
	AllClientIDs []int

	ListenConnections map[int]net.Conn
	DialConnections  map[int]net.Conn

	FailureDetectorStruct        *detector.EvtFailureDetector
	LeaderDetectorStruct         *detector.MonLeaderDetector
	Leader                      int
	Subscriber             <-chan int
	BreakOut						chan bool
	PermissionForNextVal    chan int

	proposer              *multipaxos.Proposer
	acceptor              *multipaxos.Acceptor
	learner               *multipaxos.Learner

	Wg                   sync.WaitGroup

	CurRound  int

	BufferedValues        map[int]*multipaxos.DecidedValue
	AccountsList          map[int]*bank.Account
	MainServer       int
	MaxSequence   int
	CatchTimeOut  bool
	TimeoutSignal *time.Ticker
	Value          multipaxos.Value

	Start            time.Time
	StartTT				time.Time
	EndTT             time.Time
	End              time.Time

	Stop          chan bool
	TimeOfOperations []float64
	NumOfOperations  int
}

var (
	OperationValue int
	AmountValue   int
	AccountNumber int
	AutomaticMode bool
)

func NewProcess() *Process {
	P := &Process{}
	P.NumOfProcesses = 6
	P.NumOfServers = 3
	P.NumOfClients = 3
	P.MainServer = -1
	P.CatchTimeOut = false
	P.MaxSequence = 0
	P.NumOfOperations = 0

	P.AllProcessIDs = []int{0,1,2,3,4,5}
	P.AllServerIDs = []int{0,1,2}
	P.AllClientIDs = []int{3,4,5}

	P.Subscriber = make(<-chan int, 10)
	P.PermissionForNextVal = make(chan int)
	P.ListenConnections = make(map[int]net.Conn)
	P.DialConnections = make(map[int]net.Conn)
	P.BreakOut = make(chan bool)

	P.Leader = 0
	P.TimeOfOperations = make([]float64, 0)

	for {   // Ask for the process ID
		P.ProcessID = GetIntFromTerminal("Specify the ID of the process ")
		if P.ProcessID < 0 || P.ProcessID > P.NumOfProcesses-1 {
			fmt.Println("This ID is not valid")
		} else {
			break
		}
	}
	var ServerOrClient int
	for {   //Determine wheter the process is a server or a client
		ServerOrClient = GetIntFromTerminal("Press 0 for server and 1 for client")
		if ServerOrClient < 0 || ServerOrClient > 1 {
			fmt.Println("You can only enter 0 or 1")
		}else{
			if ServerOrClient == 0{
				P.Server = true
				P.Client = false
			}else{
				P.Server = false
				P.Client = true
			}
			break
		}
	}
	return P
}
func (P *Process) ListenForConnections() {
	wg.Add(1)
		go func() {
			listener, err := net.Listen("tcp", Address[P.ProcessID])   //(1200,1201,1202,1203,1204,1205)
			CheckError(err)
			for i := 0; i < P.NumOfProcesses; i++ {
				if P.Client && i >= P.NumOfServers{ // makes sure the clients anly have connections with the servers
					continue
				}
				fmt.Println("Port Number : ", P.ProcessID," listening ...")
				conn, AcErr := listener.Accept()  //listens for connections
				CheckError(AcErr)
				incomingMess := IncomingInitialMessage{}
				err2 := json.NewDecoder(conn).Decode(&incomingMess)
				if err2 != nil {
					fmt.Println("The initial message was not received! ", err2.Error())
				}
				remoteProcID := incomingMess.Pid
				if AcErr == nil {
					P.ListenConnections[remoteProcID] = conn
					fmt.Println("Listen.Connected with remote id :", remoteProcID)
				}
			}
			wg.Done()
		}()
}
func (P *Process) DialForConnections() {
	for i := 0  ; i < P.NumOfProcesses; i++ {
		if P.Client && i >= P.NumOfServers{ // makes sure the clients anly have connections with the servers
			continue
		}
		for {
			fmt.Println("Dialing to procces: ", i)
			Conn, err := net.Dial("tcp", Address[i])
			if err == nil {
				P.DialConnections[i] =  Conn
				initialMsg := IncomingInitialMessage{Pid: P.ProcessID}
				err2 := json.NewEncoder(Conn).Encode(initialMsg)
				CheckError(err2)
				fmt.Println("Dialed ", i, " and sent my id ", P.ProcessID)
				break
			}
		}
	}
}
func (P *Process) SendMessageToAllServers(M Msg) {
	for _, key := range P.AllServerIDs{
		if conn, ok := P.DialConnections[key]; ok {
			err := gob.NewEncoder(conn).Encode(&M)
			if err != nil {
				fmt.Println(err.Error())
			}
		}
	}
}
func (P *Process) SendMessageToAllClients(M Msg) {
	for _, key := range P.AllClientIDs{
		if conn, ok := P.DialConnections[key]; ok {
			err := gob.NewEncoder(conn).Encode(&M)
			if err != nil {
				fmt.Println(err.Error())
			}
		}
	}
}
func (P *Process) SendMessageToId(M Msg, id int) {      // sends a messege to a connection specified by the id
	for _, key := range P.AllProcessIDs{
		if id == key {
			if conn, ok := P.DialConnections[key]; ok {
				err1 := gob.NewEncoder(conn).Encode(&M)
				if err1 != nil {
					fmt.Println(err1.Error())
				}
			}
		}
	}
}
func (P *Process) UpdateSubscribe() {  //wait for some write in the subscriber channel in order to print out the new leader
	for {
		time.Sleep(time.Millisecond * 500)
		select {
		case gotLeader := <-P.Subscriber:
			P.Leader = gotLeader
			// Got publication, change the leader
			fmt.Println("------ NEW LEADER: ", P.Leader, "------")
		}
	}
}
func (P *Process) SendHeartbeats(){ // create a slice of heartbeats to send to the other connected processes
	for {
		for _, key := range P.AllServerIDs{ //creating heartbeat for each connection
			heartbeat := detector.Heartbeat{To: key , From: P.ProcessID, Request: true}
			var network bytes.Buffer
			message := Msg{ Msg: make([]byte,2000),}

			err := gob.NewEncoder(&network).Encode(heartbeat)
			CheckError(err)

			message.Msg = network.Bytes()
			message.MsgType = "Heartbeat"   // type to verify connection
			message.Pid = P.ProcessID

			P.SendMessageToId(message, key)
			time.Sleep(time.Millisecond * 200)
		}
		time.Sleep(time.Millisecond * 2000)
	}
}

func (P *Process) ReceiveClientValues() {
	for _, key := range P.AllClientIDs{
		wg.Add(1)
		go func(key int) {
			for {
				newMsg := Msg{}
				err := gob.NewDecoder(P.ListenConnections[key]).Decode(&newMsg)
				if err != nil {
					if SocketIsClosed(err) {
						P.CloseTheConnection(key, P.ListenConnections[key])
						break
					}
				}

/*CV*/		if newMsg.MsgType == "ClientValue" {    //Handle the incoming client value
					var value multipaxos.Value
					buff := bytes.NewBuffer(newMsg.Msg)
					err = gob.NewDecoder(buff).Decode(&value)
					CheckError(err)

					RecieveColor := color.New(color.Bold, color.FgBlue).PrintlnFunc()
					RecieveColor("Recieved value:", value, " from process :", newMsg.Pid )

					if P.Leader == -1 {  //if there is no leader chosen yet
						continue
					}
					if P.ProcessID == P.Leader{
						P.proposer.DeliverClientValue(value)

						//<-P.PermissionForNextVal
					}else{
						MsgReply := Msg{}
						var network bytes.Buffer
						err := gob.NewEncoder(&network).Encode(value)
						if err != nil {
							fmt.Println(err.Error())
						}
						MsgReply.MsgType = "Redirect"
						MsgReply.Msg = network.Bytes()
						MsgReply.Pid = P.ProcessID
						MsgReply.RedirectTo = P.Leader

						P.SendMessageToId(MsgReply, newMsg.Pid)
					}
				}
			}
			wg.Done()
		}(key)
	}
}

func (P *Process) ReceiveMessagesSendReply(hbResponse chan detector.Heartbeat) {
	for _, key := range P.AllServerIDs{
		wg.Add(1)
		go func(key int) {
			for {
				newMsg := Msg{}
				err := gob.NewDecoder(P.ListenConnections[key]).Decode(&newMsg)
				if err != nil {
					if SocketIsClosed(err) {
						P.CloseTheConnection(key, P.ListenConnections[key])
						break
					}
				}

	/*hb*/	if newMsg.MsgType == "Heartbeat" {   // heartbeat type of message
					var heartbeat detector.Heartbeat
					buff:=bytes.NewBuffer(newMsg.Msg)
					err = gob.NewDecoder(buff).Decode(&heartbeat)
					CheckError(err)
					P.FailureDetectorStruct.DeliverHeartbeat(heartbeat)
					if heartbeat.Request == true {
				     	select {
						case hbResp := <-hbResponse:
							MsgReply:= Msg{}
							var network bytes.Buffer
							err := gob.NewEncoder(&network).Encode(hbResp)
							if err != nil {
								fmt.Println(err.Error())
							}
							MsgReply.MsgType = "Heartbeat"
							MsgReply.Msg = network.Bytes()
							err1 := gob.NewEncoder(P.DialConnections[key]).Encode(&MsgReply)
							if err1 != nil {
								fmt.Println(err1.Error())
							}
						}
					}
				}

/*CV*/		if newMsg.MsgType == "ClientValue" {    //Handle the incoming client value
					var value multipaxos.Value
					buff := bytes.NewBuffer(newMsg.Msg)
					err = gob.NewDecoder(buff).Decode(&value)
					CheckError(err)

					RecieveColor := color.New(color.Bold, color.FgBlue).PrintlnFunc()
					RecieveColor("Recieved value:", value, " from process :", newMsg.Pid )

					if P.Leader == -1 {  //if there is no leader chosen yet
						continue
					}
					if P.ProcessID == P.Leader{
						P.proposer.DeliverClientValue(value)
					}else{
						MsgReply := Msg{}
						var network bytes.Buffer
						err := gob.NewEncoder(&network).Encode(value)
						if err != nil {
							fmt.Println(err.Error())
						}
						MsgReply.MsgType = "Redirect"
						MsgReply.Msg = network.Bytes()
						MsgReply.Pid = P.ProcessID
						MsgReply.RedirectTo = P.Leader

						P.SendMessageToId(MsgReply, newMsg.Pid)

					}
				}

/*PRP*/		if newMsg.MsgType == "Prepare" {
					var prepare multipaxos.Prepare
					buff := bytes.NewBuffer(newMsg.Msg)
					err = gob.NewDecoder(buff).Decode(&prepare)
					CheckError(err)
					P.acceptor.DeliverPrepare(prepare)
				}


				if newMsg.MsgType == "Promise" {
					var promise multipaxos.Promise
					buff := bytes.NewBuffer(newMsg.Msg)
					err = gob.NewDecoder(buff).Decode(&promise)
					CheckError(err)
					P.proposer.DeliverPromise(promise)
			 	}

				if newMsg.MsgType == "Accept" {
					var accept multipaxos.Accept
					buff := bytes.NewBuffer(newMsg.Msg)
					err = gob.NewDecoder(buff).Decode(&accept)
					CheckError(err)
					P.acceptor.DeliverAccept(accept)
			 	}

				if newMsg.MsgType == "Learn" {
					var learn multipaxos.Learn
					buff := bytes.NewBuffer(newMsg.Msg)
					err = gob.NewDecoder(buff).Decode(&learn)
					CheckError(err)
					P.learner.DeliverLearn(learn)
			 	}
			}
			wg.Done()
		}(key)
	}
}

func (P *Process) PaxosMethod(prepareOut chan multipaxos.Prepare,acceptOut chan multipaxos.Accept, promiseOut chan multipaxos.Promise,learnOut chan multipaxos.Learn, decidedOut chan multipaxos.DecidedValue) {
	for {
		select{
			case prp := <-prepareOut:
				var network bytes.Buffer
				err := gob.NewEncoder(&network).Encode(prp)
				CheckError(err)
				MsgReply := Msg{ MsgType : "Prepare", Msg : network.Bytes(), Pid : P.ProcessID}
				P.SendMessageToAllServers(MsgReply) //broadcast the prepare message

			case prm := <-promiseOut:
				var network bytes.Buffer
				err := gob.NewEncoder(&network).Encode(prm)
				CheckError(err)
				MsgReply := Msg{MsgType : "Promise", Msg : network.Bytes(), Pid : P.ProcessID }
				P.SendMessageToId(MsgReply, prm.To) //Send the promise only to the sender of propose m

			case acc := <-acceptOut:

				var network bytes.Buffer
				err := gob.NewEncoder(&network).Encode(acc)
				CheckError(err)
				MsgReply := Msg{MsgType : "Accept", Msg : network.Bytes(), Pid : P.ProcessID }
				P.SendMessageToAllServers(MsgReply)             //Send the accept message to everyone

			case lrn := <-learnOut:
				var network bytes.Buffer
				err := gob.NewEncoder(&network).Encode(lrn)
				CheckError(err)
				MsgReply := Msg{MsgType : "Learn", Msg : network.Bytes(), Pid : P.ProcessID}
				P.SendMessageToAllServers(MsgReply) //broadcast the learn message

			case decidedVal := <-decidedOut:
			// handle the decided value (all the servers, not only the leader)
				DecidedColor := color.New(color.Bold, color.FgRed).PrintlnFunc()
				DecidedColor("Multipaxos: Got the Decided Value ", decidedVal )
				P.HandleDecideValue(decidedVal)

		}
	}
}

func (P *Process) HandleDecideValue(decidedVal multipaxos.DecidedValue) {
	fmt.Println("Im in HandleDecideValue")
	adu := P.proposer.GetADU()
	if decidedVal.SlotID > adu+1 {
		P.BufferedValues[int(adu)+1] = &decidedVal
		return
	}

	if !decidedVal.Value.Noop {
		if P.AccountsList[decidedVal.Value.AccountNum] == nil {
			// create and store new account with a balance of zero
			P.AccountsList[decidedVal.Value.AccountNum] = &bank.Account{Number: decidedVal.Value.AccountNum, Balance: 0}
		}
		// apply transaction from value to account
		TransactionResult := P.AccountsList[decidedVal.Value.AccountNum].Process(decidedVal.Value.Txn)

		if P.ProcessID == P.Leader {
			// create response with appropriate transaction result, client id and client seq
			Response := multipaxos.Response{ClientID: decidedVal.Value.ClientID, ClientSeq: decidedVal.Value.ClientSeq, TxnRes: TransactionResult}
			MsgReply := Msg{}
			var network bytes.Buffer
			err := gob.NewEncoder(&network).Encode(Response)
			if err != nil {
				fmt.Println(err.Error())
			}
			MsgReply.MsgType = "ResponseToClient"
			MsgReply.Msg = network.Bytes()
			MsgReply.Pid = P.ProcessID
			ClientID, _ := strconv.Atoi(decidedVal.Value.ClientID)
			P.SendMessageToId(MsgReply, ClientID)

		}
	}
	// increment adu by 1 (increment decided slot for proposer)
	P.proposer.IncrementAllDecidedUpTo()
	if P.BufferedValues[int(adu)+1] != nil {
		P.HandleDecideValue(*P.BufferedValues[int(adu)+1])
	}
}

func main(){

	Proc:= NewProcess()
	Proc.ListenForConnections()
	Proc.DialForConnections()
	wg.Wait()
	Proc.PrintConnectionInformation()

	wg.Add(2)
	if Proc.Server {

		Proc.BufferedValues = make(map[int]*multipaxos.DecidedValue)
		Proc.AccountsList = make(map[int]*bank.Account)

		// add the leader Detector Struct
		Proc.LeaderDetectorStruct = detector.NewMonLeaderDetector(Proc.AllServerIDs) //Call the Leader detector only on servers
		Proc.Leader = Proc.LeaderDetectorStruct.Leader()
		fmt.Println("------LEADER IS PROCESS: ", Proc.Leader,"------"); fmt.Println("")

		//Subscribe for leader changes
		Proc.Subscriber = Proc.LeaderDetectorStruct.Subscribe()
		go Proc.UpdateSubscribe()    // wait to update the subscriber

		// Add the eventual failure detector
		hbResponse := make(chan detector.Heartbeat, 10) //channel to communicate with fd
		Proc.FailureDetectorStruct = detector.NewEvtFailureDetector(Proc.ProcessID, Proc.AllServerIDs, Proc.LeaderDetectorStruct, delta, hbResponse)
		Proc.FailureDetectorStruct.Start()

		//Paxos Implementation
		prepareOut := make(chan multipaxos.Prepare, 10)
		acceptOut := make(chan multipaxos.Accept, 10)
		promiseOut := make(chan multipaxos.Promise, 10)
		learnOut := make(chan multipaxos.Learn, 10)
		decidedOut := make(chan multipaxos.DecidedValue, 10)

		wg.Add(4)
		Proc.ReceiveMessagesSendReply(hbResponse)
		Proc.ReceiveClientValues()

		go Proc.PaxosMethod(prepareOut, acceptOut, promiseOut, learnOut, decidedOut)

		Proc.proposer = multipaxos.NewProposer(Proc.ProcessID, Proc.NumOfServers, -1, Proc.LeaderDetectorStruct, prepareOut, acceptOut)
		Proc.proposer.Start()
		Proc.acceptor = multipaxos.NewAcceptor(Proc.ProcessID, promiseOut, learnOut)
		Proc.acceptor.Start()
		Proc.learner = multipaxos.NewLearner(Proc.ProcessID, Proc.NumOfServers, decidedOut)
		Proc.learner.Start()


		go Proc.SendHeartbeats()
		wg.Wait()
	}

	if Proc.Client {
		// choose between manual and automatic mode
		Mode := TerminalInput("Manual mode (m) or Automatic mode (a)? ")
		Proc.ReceivedValue = make(chan bool, 10)
		Proc.Stop = make(chan bool, 10)

		if Proc.MainServer == -1 { // if the main server was not chosen
			Proc.MainServer = RandomFromSlice(Proc.AllServerIDs)
			fmt.Println("Current Main Server:",Proc.MainServer )
		}

		wg.Add(2)
		Proc.GetTheValue()

		if Mode == "m" {
			AutomaticMode = false
		} else if Mode == "a" {
			AutomaticMode = true
		}

		go Proc.WaitForTimeout()
		Proc.StartGenerator()

		wg.Wait()
	}
	wg.Wait()
}

func (P *Process) WaitForTimeout() {
	P.TimeoutSignal = time.NewTicker(time.Second * 10)
	Warning := color.New(color.FgRed).PrintfFunc()
	for {
		select{
		case <- P.ReceivedValue:
			P.GenerateTheValue()

		case <- P.TimeoutSignal.C:
			if P.CatchTimeOut {
				Warning("*****Got Timout*****"); fmt.Println("")
				if _, ok := P.DialConnections[P.MainServer]; !ok {   //Check if the MainServer is alive, if not then choose another server randomly
					Warning("Main Server is not alive, choosing a new server randlomy "); fmt.Println("")
					P.MainServer = RandomFromSlice(P.AllServerIDs)
				}
				P.SendValue()  //Resend the value
			}

		case <- P.Stop :
			//Print out the stats
			time.Sleep(time.Second * 1)
			mean, _ := stats.Mean(P.TimeOfOperations)
			minimum, _ := stats.Min(P.TimeOfOperations)
			maximum, _ := stats.Max(P.TimeOfOperations)
			median, _ := stats.Median(P.TimeOfOperations)
			variance, _ := stats.Variance(P.TimeOfOperations)
			stddev := math.Sqrt(variance)
			percentile, _ := stats.Percentile(P.TimeOfOperations, 99)


			Info := color.New(color.Bold, color.FgRed).PrintlnFunc()
			Info("Mean in seconds: ", mean )
			Info("Minimum in seconds:", minimum )
			Info("Maximum in seconds:", maximum )
			Info("Median in seconds: ", median )
			Info("Standard deviation in seconds: ", stddev )
			Info("99 Percentile in seconds: ", percentile )

		}
	}
}

func (P *Process) GenerateTheValue() {
	if AutomaticMode {
		P.Start = time.Now()
		OperationValue = rand.Intn(3)
		AccountNumber = rand.Intn(1000)
		AmountValue = 0
		if OperationValue > 0 {
			AmountValue = rand.Intn(10000)
		}
	}else {
		OperationValue = GetIntFromTerminal("Specify the operation (0: Balance, 1: Deposit, 2: Withdrawal) ")
		AccountNumber = GetIntFromTerminal("Insert the account number ")
		AmountValue = 0
		if OperationValue > 0 {
			AmountValue = GetIntFromTerminal("Insert the amount ")
		}
	}

	ClientID := strconv.Itoa(P.ProcessID)
	transaction := bank.Transaction{Op: bank.Operation(OperationValue), Amount: AmountValue}
	P.Value = multipaxos.Value{ClientID: ClientID, ClientSeq: P.MaxSequence, AccountNum: AccountNumber, Txn: transaction}

	P.SendValue()
}

func (P *Process) SendValue(){
	P.TimeoutSignal = time.NewTicker(time.Second * 10)
	P.CatchTimeOut = true

	var network bytes.Buffer
	err := gob.NewEncoder(&network).Encode(P.Value)
	CheckError(err)
	message := Msg{Msg : network.Bytes(), MsgType : "ClientValue", Pid : P.ProcessID  }
	P.SendMessageToId(message, P.MainServer)
	fmt.Println("Sent Value :", P.Value, " To Server : ", P.MainServer)
}

func  (P *Process) GetTheValue() {
	for _, key := range P.AllServerIDs{
		wg.Add(1)
		go func(key int) {
			for {
				newMsg := Msg{}
				err := gob.NewDecoder(P.ListenConnections[key]).Decode(&newMsg)
				if err != nil {
					if SocketIsClosed(err) {
						P.CloseTheConnection(key, P.ListenConnections[key])
						break
					}
				}
/*RV*/		if newMsg.MsgType == "ResponseToClient" {    //Handle the incoming client value
					P.CatchTimeOut = false //signal for the client to ignore the timout since we got the value
					P.End = time.Now()

					var response multipaxos.Response
					buff := bytes.NewBuffer(newMsg.Msg)
					err = gob.NewDecoder(buff).Decode(&response)
					CheckError(err)

					ResponseColor := color.New(color.Bold, color.FgGreen).PrintlnFunc()
					ResponseColor("Recieved response:", response, " from server :", newMsg.Pid ); fmt.Println("")

					P.TimeOfOperations = append(P.TimeOfOperations, P.End.Sub(P.Start).Seconds())
					P.MaxSequence++
					P.NumOfOperations ++

					if AutomaticMode{
						if P.NumOfOperations < 500{
							P.StartGenerator()
						}else{
							P.StopGenerator()
						}
					}else{
						P.StartGenerator()
					}
				}

/*R	*/		if newMsg.MsgType == "Redirect" {    //Handle the redirect message
					P.MainServer = newMsg.RedirectTo
					var value multipaxos.Value
					buff := bytes.NewBuffer(newMsg.Msg)
					err = gob.NewDecoder(buff).Decode(&value)
					CheckError(err)
					fmt.Println("Recieved redirect to server :", newMsg.RedirectTo, " from server :", newMsg.Pid, "with value : ", value )
					fmt.Println("The main server is now :", P.MainServer)

					var network bytes.Buffer                // and send the value to the correct server according to the redirect message
					err := gob.NewEncoder(&network).Encode(value)
					CheckError(err)
					message := Msg{ Msg: network.Bytes(), MsgType : "ClientValue", Pid : P.ProcessID }
					P.SendMessageToId(message, P.MainServer)
					fmt.Println("Sent Value :", value, " To Server : ", P.MainServer)
				}
			}
			wg.Done()
		}(key)
	}
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
func (P *Process) StopGenerator() {
	P.Stop <- true // stop the automatic send
}
func (P *Process) StartGenerator() {
	P.ReceivedValue <- true
}
func (P *Process) ResetTicker() {
    P.TimeoutSignal = time.NewTicker(time.Second * 10)
}
func RandomFromSlice(slice []int) int {
    rand.Seed(time.Now().Unix())
    message := slice[rand.Intn(len(slice))]
    return message
}
func (P *Process) PrintConnectionInformation(){
		fmt.Println("------CONNECTIONS ESTABLISHED------")
		fmt.Println("Listen conns : ", P.ListenConnections)
		fmt.Println("Dial conns : ",P.DialConnections)
}
func SocketIsClosed(err error) bool {
	if strings.Contains(err.Error(), "EOF") || strings.Contains(err.Error(), "forcibly closed by the remote host") {
		return true
	}
	return false
}
func (P *Process) CloseTheConnection(index int, conn net.Conn) {
	conn.Close()
	delete(P.ListenConnections, index)
	delete(P.DialConnections, index)

	P.AllProcessIDs = append(P.AllProcessIDs[:index], P.AllProcessIDs[index+1:]...)
	P.AllServerIDs = append(P.AllServerIDs[:index], P.AllServerIDs[index+1:]...)

	if P.Leader == index{  //if the leader died exclude him from the leadership
		P.Leader = -1
	}

	IamServer := PresenceInSlice(P.AllServerIDs, P.ProcessID)
	if IamServer{
		fmt.Println("Listen conns after disconnect", P.ListenConnections)
		fmt.Println("Dial conns after disconnect", P.DialConnections)
		fmt.Println("All process ids :",P.AllProcessIDs)
		fmt.Println("All server ids :",P.AllServerIDs)
		fmt.Println("All client ids :",P.AllClientIDs)

		fmt.Println("******CLOSED CONNECTION WITH PROCESS: ", index,"******")
	}
}
func PresenceInSlice(slice []int, id int) bool{
	output := false
	for _, key := range slice{
		if id == key {
			output = true
		}
	}
	return output
}
func CheckError(err error) {
    if err != nil {
        fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
    }
}
func TerminalInput(message string) string {
	var input string
	fmt.Print(message, ": ")
	fmt.Scanln(&input)
	return input
}
func GetIntFromTerminal(message string) int {
	s := TerminalInput(message)
	x, _ := strconv.ParseInt(s, 10, 32)
	return int(x)
}
