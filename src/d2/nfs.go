package main

import (
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

const welcomeMessage = "220 Welcome to the FTP server\r\n"

var transferMode string = "I"

var activeDataConnection net.Conn

var allowedPaths = []string{"/Users/masonwright/Desktop/actions", "/Users/masonwright/Desktop/teko", "/Users/masonwright/Documents"}

func main() {
	err := os.Chdir("/")
	if err != nil {
		log.Println("Error changing directory:", err)
	}

	listener, err := net.Listen("tcp", ":21")
	if err != nil {
		log.Fatal("Error starting FTP server:", err)
		return
	}

	log.Println("FTP server listening on port 21")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}

		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	fmt.Fprint(conn, welcomeMessage)
	activeDataConnection = conn

	for {
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				log.Println("Connection closed by client.")
				return
			}
			log.Println("Error reading command:", err)
			return
		}

		command := strings.TrimSpace(string(buf[:n]))

		log.Println("Received command:", command)
		response := processCommand(conn, command)

		if len(response) > 0 {
			_, err = fmt.Fprint(conn, response)
			if err != nil {
				log.Println("Error writing response:", err)
				return
			}
		}
	}
}

func processCommand(conn net.Conn, command string) string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "500 Invalid command\r\n"
	}

	switch parts[0] {
	case "USER":
		return "331 User name okay, need password\r\n"
	case "PASS":
		return "230 User logged in\r\n"
	case "LIST":
		return handleListCommand(conn)
	case "SYST":
		return generateSYSTResponse()
	case "STAT":
		return generateSTATResponse(parts[1])
	case "PWD":
		return handlePWDCommand()
	case "TYPE":
		return handleTypeCommand(parts)
	case "CWD":
		return handleCWDCommand(parts)
	case "PASV":
		return handlePASVCommand(conn)
	case "PORT":
		return handlePORTCommand(conn, parts)
	case "RETR":
		return handleRETRCommand(conn, strings.Join(parts[1:], " "))
	case "MKD":
		return createDirectory(conn, strings.Join(parts[1:], " "))
	case "RMD":
		return removeDirectory(conn, strings.Join(parts[1:], " "))
	case "DELE":
		return deleteFile(conn, strings.Split(strings.Join(parts[1:], " "), "\n"))
	case "QUIT":
		return "221 Service closing control connection\r\n"
	default:
		return "500 Unknown command\r\n"
	}
}

func whitelist(list []string, path string) bool {
	clnPath := strings.Split(strings.Trim(path, "/"), "/")

	for _, v := range list {
		item := strings.Split(strings.Trim(v, "/"), "/")

		// Check if item is a prefix of clnPath
		if len(item) <= len(clnPath) {
			isPrefix := true
			for i := 0; i < len(item); i++ {
				if item[i] != clnPath[i] {
					isPrefix = false
					break
				}
			}
			if isPrefix {
				return true
			}
		} else {
			isPrefix := true
			for i := 0; i < len(clnPath); i++ {
				if item[i] != clnPath[i] {
					isPrefix = false
					break
				}
			}
			if isPrefix {
				return true
			}
		}
	}

	return false
}

func generateSYSTResponse() string {
	// Get the server's operating system
	os := runtime.GOOS

	// Map common operating systems to their corresponding FTP codes
	osCode := map[string]string{
		"linux":   "L8",
		"darwin":  "L8",
		"windows": "WIN32",
	}

	// Use "UNKNOWN" if the operating system is not in the map
	code, ok := osCode[os]
	if !ok {
		code = "UNKNOWN"
	}

	return fmt.Sprintf("215 %s Type: %s\r\n", "UNIX", code)
}

func generateSTATResponse(directoryPath string) string {
	// Get the list of files in the specified directory
	files, err := ioutil.ReadDir(directoryPath)
	if err != nil {
		fmt.Println("Error listing directory:", err)
		return "500 Error listing directory\r\n"
	}

	statResponse := unixLS(files, directoryPath)

	println(statResponse)
	// Convert the STAT response to bytes
	return "213-Status follows:\r\n" + statResponse + "213 End of status\r\n"
}

func unixLS(files []fs.FileInfo, dir string) string {

	var output strings.Builder

	for _, file := range files {
		if whitelist(allowedPaths, filepath.Join(dir, file.Name())) {
			// Format the modification time
			modTime := file.ModTime().Format("Jan 02 15:04")

			// Determine the entry type (directory, file, or link)
			entryType := " "
			if file.IsDir() {
				entryType = "d"
			} else if file.Mode()&os.ModeSymlink != 0 {
				entryType = "l"
			}

			entry := ""
			if entryType == "d" {

				// Build the entry string
				entry = " drwxrwxr-x" + fmt.Sprintf(" %3d %8d %12d %s %s\r\n",
					1, file.Sys().(*syscall.Stat_t).Uid, file.Size(), modTime, file.Name())
			} else {
				// Build the entry string
				entry = fmt.Sprintf("%s%s %3d %8d %12d %s %s\r\n",
					entryType, file.Mode().Perm(), 1, file.Sys().(*syscall.Stat_t).Uid, file.Size(), modTime, file.Name())

			}

			// Append the entry to the STAT response
			output.WriteString(entry)
		}

	}

	println(output.String())
	return output.String()
}

func generateCurrentDirectoryListing() string {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting working directory:", err)
	}
	// Get the list of files in the current directory
	files, err := ioutil.ReadDir(".")
	if err != nil {
		fmt.Println("Error listing directory:", err)
		return "nil"
	}

	// Convert the directory listing to bytes
	return unixLS(files, wd)
}

func sendFileOverDataConnection(dataConn net.Conn, filePath string) {
	println("FILE: ", filePath)
	// Open the file for reading
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	// Copy file data to the data connection based on the transfer mode
	var reader io.Reader
	if transferMode == "A" {
		reader = transform.NewReader(file, charmap.Windows1252.NewEncoder())
	} else {
		reader = file
	}

	if err != nil {
		fmt.Println("Error sending response to client:", err)
		return
	}

	// Copy file data to the data connection
	_, err = io.Copy(dataConn, reader)
	if err != nil {
		fmt.Println("Error sending file data:", err)
		return
	}

	fmt.Println("File sent successfully.")
}

// Function to store a file
func storeFile(conn net.Conn, fileName string) {
	file, err := os.Create(fileName)
	if err != nil {
		fmt.Println("Error creating file:", err)
		conn.Close()
		return
	}
	defer file.Close()

	_, err = io.Copy(file, conn)
	if err != nil {
		fmt.Println("Error storing file:", err)
		conn.Close()
		return
	}

	fmt.Println("File stored successfully.")
	conn.Close()
}

// Function to append data to a file
func appendFile(conn net.Conn, fileName string) {
	file, err := os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening file for append:", err)
		conn.Close()
		return
	}
	defer file.Close()

	_, err = io.Copy(file, conn)
	if err != nil {
		fmt.Println("Error appending data to file:", err)
		conn.Close()
		return
	}

	fmt.Println("Data appended to file successfully.")
	conn.Close()
}

// Function to delete a file
func deleteFile(conn net.Conn, fileName []string) string {
	println(fileName)
	for _, v := range fileName {
		err := os.Remove(v)
		if err != nil {
			fmt.Println("Error deleting file:", err)
		}
	}
	return "200 File deleted successfully."
}

// Function to create a directory
func createDirectory(conn net.Conn, dirName string) string {
	err := os.Mkdir(dirName, os.ModePerm)
	// Respond with appropriate status code based on success or failure
	if err != nil {
		return "550 Requested action not taken\r\n"
	}
	return "257 \"" + dirName + "\" created\r\n"
}

// Function to remove a directory
func removeDirectory(conn net.Conn, dirName string) string {
	err := os.Remove(dirName)
	if err != nil {
		fmt.Println("Error removing directory:", err)
	}
	return "200 Directory removed successfully."
}

// Add other command handling functions here...

func handlePWDCommand() string {
	wd, err := os.Getwd()
	if err != nil {
		log.Println("Error getting working directory:", err)
		return "550 Requested action not taken\r\n"
	}
	return fmt.Sprintf("257 \"%s\" is the current directory\r\n", wd)
}

func handleListCommand(dataConn net.Conn) string {
	defer activeDataConnection.Close()
	fmt.Fprint(dataConn, "150 Here comes the directory listing\r\n")
	// Send the data over the data connection
	listing := generateCurrentDirectoryListing()

	println(listing)
	// activeDataConnection.Write([]byte(listing))
	fmt.Fprint(activeDataConnection, listing)

	fmt.Fprint(dataConn, "226 Directory listing successful.\r\n")

	fmt.Println("Directory listing sent successfully.")
	return ""
}

func handleRETRCommand(conn net.Conn, parts string) string {
	defer activeDataConnection.Close()
	fmt.Fprint(conn, "150 File status okay; about to open data connection\r\n")
	sendFileOverDataConnection(activeDataConnection, parts)
	fmt.Fprint(conn, "226 Closing data connection. Requested file action successful\r\n")

	return ""
}

func handleTypeCommand(parts []string) string {
	if len(parts) != 2 {
		return "501 Syntax error in parameters or arguments\r\n"
	}
	// Support both ASCII (A) and Binary (I) modes
	if strings.ToUpper(parts[1]) == "A" || strings.ToUpper(parts[1]) == "I" {
		transferMode = strings.ToUpper(parts[1])
		return "200 Command okay\r\n"
	}
	return "504 Command not implemented for that parameter\r\n"
}

func handleCWDCommand(parts []string) string {
	if len(parts) != 2 {
		return "501 Syntax error in parameters or arguments\r\n"
	}
	err := os.Chdir(parts[1])
	if err != nil {
		log.Println("Error changing directory:", err)
		return "550 Requested action not taken\r\n"
	}
	return "250 Requested file action okay, completed\r\n"
}

func handlePORTCommand(conn net.Conn, parts []string) string {
	println(parts)
	dataConn, err := setupActiveDataConnection(parts[1])
	if err != nil {
		log.Println("Error setting up active data connection:", err)
		return "425 Can't open data connection\r\n"
	}
	activeDataConnection = dataConn

	return "200 PORT command successful\r\n"
}

func handlePASVCommand(conn net.Conn) string {
	listener, err := setupPassiveDataConnection(conn)
	if err != nil {
		log.Println("Error setting up passive data connection:", err)
		return "425 Can't open data connection\r\n"
	}

	// Accept a connection from the client
	dataConn, err := listener.Accept()
	if err != nil {
		log.Println("Error accepting passive data connection:", err)
		return "425 Can't open data connection\r\n"
	}
	activeDataConnection = dataConn

	return ""
}

func setupPassiveDataConnection(conn net.Conn) (net.Listener, error) {
	listener, err := net.Listen("tcp", ":0") // Use port 0 to automatically select an available port
	if err != nil {
		return nil, err
	}

	addr := listener.Addr().(*net.TCPAddr)
	ipParts := strings.Split(strings.Split(conn.LocalAddr().String(), ":")[0], ".") // Obtain the server's IPv4 address
	portParts := []string{
		strconv.Itoa(addr.Port / 256),
		strconv.Itoa(addr.Port % 256),
	}

	fmt.Printf("Passive IP: %s, Passive Port: %d\n", ipParts, addr.Port)

	// Pad with a zero if the value is less than 256
	if len(portParts) == 1 {
		portParts = append(portParts, "0")
	}

	// Ensure ipParts has at least four elements
	for len(ipParts) < 4 {
		ipParts = append(ipParts, "0")
	}

	response := fmt.Sprintf("227 Entering Passive Mode. %s,%s,%s,%s,%s,%s\r\n",
		ipParts[0], ipParts[1], ipParts[2], ipParts[3],
		portParts[0], portParts[1])
	log.Println(response)

	// Send the PASV response to the client
	_, err = fmt.Fprint(conn, response)
	if err != nil {
		fmt.Println("Error writing PASV response:", err)
		return nil, err
	}

	return listener, nil
}

func setupActiveDataConnection(addr string) (net.Conn, error) {
	parts := strings.Split(addr, ",")
	if len(parts) != 6 {
		return nil, fmt.Errorf("501 syntax error in parameters or arguments")
	}

	ip := strings.Join(parts[:4], ".")
	highByte, err := strconv.Atoi(parts[4])
	if err != nil {
		return nil, err
	}
	lowByte, err := strconv.Atoi(parts[5])
	if err != nil {
		return nil, err
	}

	port := (highByte << 8) + lowByte

	dataConn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		return nil, err
	}

	return dataConn, nil
}
