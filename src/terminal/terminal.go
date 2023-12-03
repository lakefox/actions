package terminal

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
)

func getShellCommand() string {
	switch runtime.GOOS {
	case "darwin":
		return `/bin/zsh`
	case "linux":
		return "/bin/bash"
	case "windows":
		return "cmd"
	default:
		// Use a default shell for other operating systems
		return "/bin/sh"
	}
}

type Output struct {
	In  chan<- string
	Out <-chan string
	Pty *os.File
}

func Terminal(dir string) Output {
	ptmx, tty, err := pty.Open()
	if err != nil {
		fmt.Println("Error opening PTY:", err)
	}

	stdinRead, stdinWrite := io.Pipe()
	stdoutRead, stdoutWrite := io.Pipe()

	// Replace os.Stdin and os.Stdout with custom streams
	go func() {
		_, _ = io.Copy(ptmx, stdinRead)
	}()

	go func() {
		_, _ = io.Copy(stdoutWrite, ptmx)
	}()

	// Example: Set terminal size
	setTermSize(ptmx.Fd(), 80, 24)

	// Wait group to wait for the command to complete
	var wg sync.WaitGroup
	wg.Add(1)

	// Start the command in a goroutine
	go func() {
		// Wait for the command to complete
		cmd := exec.Command(getShellCommand())
		cmd.Stdin = tty
		cmd.Stdout = tty
		cmd.Stderr = tty

		err := cmd.Run()
		if err != nil {
			fmt.Println("Error running command:", err)
		}

		// Signal that the command has completed
		wg.Done()
	}()

	// Output channel to receive data from the terminal
	outputCh := make(chan string)

	// Goroutine to read data from the PTY and send it to the output channel
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdoutRead.Read(buf)
			if err != nil {
				if err == io.EOF {
					break
				}
				fmt.Println("Error reading from PTY:", err)
				break
			}
			outputCh <- string(buf[:n])
		}
		close(outputCh)
	}()

	// Input channel to send data to the terminal
	inputCh := make(chan string)

	// Goroutine to send data from the input channel to the PTY
	go func() {
		for {
			select {
			case data, ok := <-inputCh:
				if !ok {
					return
				}
				_, _ = stdinWrite.Write([]byte(data))
			}
		}
	}()

	inputCh <- "cd " + dir + "\n"
	inputCh <- "setopt PROMPT_CR && setopt PROMPT_SP && export PROMPT_EOL_MARK=\"\" && clear\n"

	// Wait for the command to complete before closing channels
	go func() {
		wg.Wait()

		// Close the PTY and pipes when the command completes
		ptmx.Close()
		tty.Close()
		stdinWrite.Close()
		stdoutWrite.Close()
	}()

	o := Output{
		In:  inputCh,
		Out: outputCh,
		Pty: ptmx,
	}

	return o
}

func (o Output) Resize(cols, rows float64) {

	setTermSize(o.Pty.Fd(), uint16(cols), uint16(rows))
}

// The pty package uses termios functions which are not available in Windows.
func setTermSize(fd uintptr, cols, rows uint16) {
	ws := &winsize{
		Row: rows,
		Col: cols,
	}
	_, _, _ = syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		syscall.TIOCSWINSZ,
		uintptr(unsafe.Pointer(ws)),
	)
}

type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}
