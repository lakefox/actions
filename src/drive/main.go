// Hellofs implements a simple "hello world" file system.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	_ "bazil.org/fuse/fs/fstestutil"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s MOUNTPOINT\n", os.Args[0])
	flag.PrintDefaults()
}

func createDirectory(path string) error {
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		return err
	}
	fmt.Printf("Directory %s created\n", path)
	return nil
}

func deleteDirectory(path string) error {
	err := os.RemoveAll(path)
	if err != nil {
		return err
	}
	fmt.Printf("Directory %s deleted\n", path)
	return nil
}

func main() {

	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 1 {
		usage()
		os.Exit(2)
	}
	mountpoint := flag.Arg(0)

	_, err := os.Stat(mountpoint)
	if os.IsNotExist(err) {
		// Create the directory when the application starts
		err := createDirectory(mountpoint)
		if err != nil {
			fmt.Printf("Error creating directory: %s\n", err)
			return
		}
	}

	c, err := fuse.Mount(
		mountpoint,
		fuse.FSName("helloworld"),
		fuse.Subtype("hellofs"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	filesys := &FS{}

	// Set up signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Increment wait group counter
	filesys.wg.Add(1)

	// Start FUSE server in a goroutine
	go func() {
		defer filesys.wg.Done()

		err := fs.Serve(c, filesys)
		if err != nil {
			log.Fatalf("failed to serve FUSE filesystem: %v", err)
		}
	}()

	// Wait for either the work to finish or a signal to be received
	select {
	case <-sigCh:
		// Handle signal, clean up, etc.
		fmt.Println("Received signal. Cleaning up...")
		// You can add additional cleanup logic here
		// ...

		// If the FUSE server is still serving, unmount and wait for it to finish
		err := fuse.Unmount(mountpoint)
		if err != nil {
			log.Fatalf("unmount failed: %v", err)
		} else {
			log.Println("unmount successful")
		}

		// Wait for the FUSE server to finish before exiting
		filesys.wg.Wait()
		fmt.Println("Program exiting.")
		return
	default:
		// Continue if no signal is received
	}

	// Wait for the FUSE server to finish before exiting
	filesys.wg.Wait()
	fmt.Println("Program exiting.")
}

// FS implements the hello world file system.
type FS struct {
	wg sync.WaitGroup
}

func (FS) Root() (fs.Node, error) {
	return Dir{}, nil
}

// Dir implements both Node and Handle for the root directory.
type Dir struct {
	RootPath string
}

func (Dir) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = 1
	a.Mode = os.ModeDir | 0o555
	return nil
}

func (Dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	println(name)
	if name == "hello" {
		return File{}, nil
	}
	return nil, syscall.ENOENT
}

var dirDirs = []fuse.Dirent{
	{Inode: 2, Name: "hello", Type: fuse.DT_File},
}

func (Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	return dirDirs, nil
}

// File implements both Node and Handle for the hello file.
type File struct{}

const greeting = "hello, world\n"

func (File) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = 2
	a.Mode = 0o444
	a.Size = uint64(len(greeting))
	return nil
}

func (File) ReadAll(ctx context.Context) ([]byte, error) {
	return []byte(greeting), nil
}
