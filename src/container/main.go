package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// go run main.go run <cmd> <args>
func main() {
	switch os.Args[1] {
	case "run":
		run()
	case "child":
		child()
	default:
		panic("help")
	}
}

func run() {
	fmt.Printf("Running %v \n", os.Args[2:])

	cmd := exec.Command("/proc/self/exe", append([]string{"child"}, os.Args[2:]...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:   syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
		Unshareflags: syscall.CLONE_NEWNS,
	}

	// Set up cgroups to limit resources (optional)
	// Uncomment the lines below if you want to limit CPU and memory
	/*
		cgroupPath := "/sys/fs/cgroup"
		err := os.Mkdir(filepath.Join(cgroupPath, "cpu-memory-limit"), 0755)
		if err != nil {
			fmt.Printf("Error creating cgroup: %v\n", err)
			os.Exit(1)
		}
		defer func() {
			os.RemoveAll(filepath.Join(cgroupPath, "cpu-memory-limit"))
		}()

		cgroups := fmt.Sprintf("cpu-memory-limit:%d", os.Getpid())
		err = ioutil.WriteFile(filepath.Join(cgroupPath, "cpu-memory-limit/tasks"), []byte(fmt.Sprintf("%d", os.Getpid())), 0700)
		if err != nil {
			fmt.Printf("Error writing to cgroup: %v\n", err)
			os.Exit(1)
		}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(1), Gid: uint32(1)}
	*/

	check(cmd.Run())
}

func child() {
	fmt.Printf("Running %v \n", os.Args[2:])

	setupContainer()

	println("s")

	cmd := exec.Command(os.Args[2], os.Args[3:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	check(cmd.Run())

	// Clean up: Unmount filesystems
	check(syscall.Unmount("proc", 0))
	check(syscall.Unmount("thing", 0))
}

func setupContainer() {
	// Specify the path for the container root filesystem
	containerRootfs := "./root"

	// Ensure the container root directory exists
	// check(os.Mkdir(containerRootfs, 0755))

	// Use debootstrap to install a minimal filesystem (adjust for your Linux distribution)
	// cmd := exec.Command("debootstrap", "stable", containerRootfs)
	// check(cmd.Run())

	println("good")

	// Mount necessary filesystems inside the container
	check(syscall.Mount("proc", filepath.Join(containerRootfs, "proc"), "proc", 0, ""))
	check(syscall.Mount("sysfs", filepath.Join(containerRootfs, "sys"), "sysfs", 0, ""))
	check(syscall.Mount("tmpfs", filepath.Join(containerRootfs, "dev"), "tmpfs", syscall.MS_NOSUID|syscall.MS_STRICTATIME, "mode=755"))

	// Set up container environment
	check(os.Chdir(containerRootfs))
	check(syscall.Chroot(containerRootfs))
	check(os.Chdir("/"))
	check(syscall.Mount("proc", "proc", "proc", 0, ""))
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
