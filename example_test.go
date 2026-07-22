package rotatelogs_test

import (
	"fmt"
	"os"

	rotatelogs "github.com/khan-lau/file-rotatelogs"
)

func ExampleForceNewFile() {
	logDir, err := os.MkdirTemp("", "rotatelogs_test")
	if err != nil {
		fmt.Println("could not create log directory ", err)

		return
	}
	logPath := fmt.Sprintf("%s/test.log", logDir)

	for i := 0; i < 2; i++ {
		writer, innerErr := rotatelogs.New(logPath, rotatelogs.ForceNewFile())
		if innerErr != nil {
			fmt.Println("Could not open log file ", innerErr)

			return
		}

		n, writeErr := writer.Write([]byte("test"))
		if writeErr != nil || n != 4 {
			fmt.Println("Write failed ", writeErr, " number written ", n)

			return
		}
		err = writer.Close()
		if err != nil {
			fmt.Println("Close failed ", err)

			return
		}
	}

	files, err := os.ReadDir(logDir)
	if err != nil {
		fmt.Println("ReadDir failed ", err)

		return
	}
	for _, file := range files {
		info, infoErr := file.Info()
		if infoErr != nil {
			fmt.Println("Info failed ", infoErr)
			continue
		}
		fmt.Println(file.Name(), info.Size())
	}

	err = os.RemoveAll(logDir)
	if err != nil {
		fmt.Println("RemoveAll failed ", err)

		return
	}
	// OUTPUT:
	// test.1.log 4
	// test.log 4
}
