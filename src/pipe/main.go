package main

import (
	"fmt"
	"os/exec"
)

func main() {
	// Create a command to execute "echo" command
	cmdEcho := exec.Command("echo", "Hello, world!")

	// Create a command to execute "grep" command with a pipe connected to cmdEcho
	cmdGrep := exec.Command("grep", "world")

	// Set up the pipe from cmdEcho to cmdGrep
	pipe, err := cmdEcho.StdoutPipe()
	if err != nil {
		fmt.Println("Error creating pipe:", err)
		return
	}
	cmdGrep.Stdin = pipe

	// Start cmdEcho
	if err := cmdEcho.Start(); err != nil {
		fmt.Println("Error starting cmdEcho:", err)
		return
	}

	// Start cmdGrep
	if err := cmdGrep.Start(); err != nil {
		fmt.Println("Error starting cmdGrep:", err)
		return
	}

	// Wait for cmdEcho and cmdGrep to finish
	if err := cmdEcho.Wait(); err != nil {
		fmt.Println("Error waiting for cmdEcho:", err)
		return
	}

	// Close the pipe to signal cmdGrep that no more data will be written
	if err := pipe.Close(); err != nil {
		fmt.Println("Error closing pipe:", err)
		return
	}

	// Wait for cmdGrep to finish
	if err := cmdGrep.Wait(); err != nil {
		fmt.Println("Error waiting for cmdGrep:", err)
		return
	}

	// Output from cmdGrep is available in cmdGrep.Stdout
	output, err := cmdGrep.Output()
	if err != nil {
		fmt.Println("Error getting cmdGrep output:", err)
		return
	}

	// Print the output
	fmt.Printf("Grep Output: %s\n", output)
}
