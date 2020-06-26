package common

import (
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/log"
)

// LogFilePath is the path of the log files
const LogFilePath = "./experiment/" // same as GOPATH/src/github.com/ethereum/go-ethereum/build/bin/experiment/

// LogToFile logs the string to the file (jmlee)
func LogToFile(fileName, logData string) {
	// open the file
	f, err := os.OpenFile(LogFilePath+fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Info("ERR", "err", err)
	}

	// log to the file
	fmt.Fprintln(f, logData)
	f.Close()
}
