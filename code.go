package main

import (
	"fmt"
	"strconv"
	"time"
)

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

func main() {

	c1 := make(chan string)
	c2 := make(chan string)

	go func() {
		time.Sleep(time.Second * 1)
		c1 <- "one"
	}()
	go func() {
		time.Sleep(time.Second * 2)
		c2 <- "two"
	}()

	for i := 0; i < 2; i++ {
		// Await both of these values
		// simultaneously, printing each one as it arrives.
		select {
		case msg1 := <-c1:
			fmt.Println("received", msg1)
		case msg2 := <-c2:
			fmt.Println("received", msg2)
		}
	}
}

// func main() {
// 	fmt.Println("Hallo world")
// 	addrs := []string{":8081", ":8082", ":8083", ":8084"}
// 	port := 8082

// 	addrs = removePort(addrs, port)
// 	fmt.Println(addrs)

// }
// }
