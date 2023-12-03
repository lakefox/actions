package actions

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type CommandRunner struct {
	In  chan string
	Out chan string
	Dir string
}

func NewCommandRunner(dir string) *CommandRunner {
	return &CommandRunner{
		In:  make(chan string),
		Out: make(chan string),
		Dir: dir,
	}
}

func (cr *CommandRunner) Run() {
	cmd := exec.Command("bash")
	cmd.Dir = cr.Dir

	// Create a pipe for command's stdin
	cmdIn, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}

	// Create a pipe for command's stdout
	cmdOut, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}

	// Start the command
	err = cmd.Start()
	if err != nil {
		panic(err)
	}

	// Read output and send it to the Out channel
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := cmdOut.Read(buf)
			if err != nil {
				if err != io.EOF {
					panic(err)
				}
				break
			}
			cr.Out <- string(buf[:n])
		}
		close(cr.Out)
	}()

	// Process commands from the In channel
	for cmdStr := range cr.In {
		_, _ = cmdIn.Write([]byte(cmdStr + "\n"))
	}
}

// EnvLoader loads environment variables from a file.
type EnvLoader struct {
	envVars map[string]string
}

// Env represents an environment with methods to load variables.
type Env struct {
	EnvLoader
}

// NewEnv creates a new Env instance.
func NewEnv() *Env {
	return &Env{
		EnvLoader: EnvLoader{
			envVars: make(map[string]string),
		},
	}
}

// LoadFromMap loads environment variables from a map.
func (e *Env) LoadFromMap(envMap map[string]string) {
	for k, v := range envMap {
		e.envVars[k] = v
	}
}

// LoadFromDir loads environment variables from a .env file in the specified directory.
func (e *Env) LoadFromDir(folderPath string) error {
	envVars, err := loadEnvFromFile(filepath.Join(folderPath, ".env"))
	if err != nil {
		return err
	}

	// Update or add values from directory
	for k, v := range envVars {
		e.envVars[k] = v
	}

	return nil
}

// ReplaceEnvVariables replaces $ variables in a string with the corresponding data from the environment.
func (e *Env) Replace(input string) string {
	// Define a regular expression to match $ variables
	re := regexp.MustCompile(`\$(\w+)`)

	// Replace $ variables with corresponding data from the environment
	result := re.ReplaceAllStringFunc(input, func(match string) string {
		varName := strings.TrimPrefix(match, "$")
		// Check the pre-loaded .env file or map
		if val, ok := e.envVars[varName]; ok {
			return val
		}
		// If not found in YAML, check the environment variables
		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
		// If still not found, return the original match
		return match
	})

	return result
}

// loadEnvFromFile loads environment variables from a .env file.
func loadEnvFromFile(filePath string) (map[string]string, error) {
	envVars := make(map[string]string)

	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return envVars, err
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			envVars[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	return envVars, nil
}

// FileLogger represents a logger that logs data to a file.
type FileLogger struct {
	filePath string
	file     *os.File
	mu       sync.Mutex
}

// NewFileLogger creates a new FileLogger with the specified file path.
func NewFileLogger(filePath string) (*FileLogger, error) {
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return &FileLogger{
		filePath: filePath,
		file:     file,
	}, nil
}

// Log appends the provided data to the log file.
func (logger *FileLogger) Log(data string) error {
	logger.mu.Lock()
	defer logger.mu.Unlock()

	_, err := logger.file.WriteString(data + "\n")
	return err
}

// Close closes the log file.
func (logger *FileLogger) Close() error {
	logger.mu.Lock()
	defer logger.mu.Unlock()

	return logger.file.Close()
}

type actions struct {
	Env     map[string]string `yaml:"env"`
	Logging struct {
		File string `yaml:"file"`
	} `yaml:"logging"`
	Actions map[string]struct {
		Desc   string   `yaml:"desc"`
		Script []string `yaml:"script"`
	} `yaml:"actions"`
}

type ActionRunner struct {
	Runner *CommandRunner
	Yaml   actions
	Env    *Env
	File   *FileLogger
}

func Action(dir string) ActionRunner {
	cr := NewCommandRunner(dir)
	go cr.Run()

	fileData, err := os.ReadFile(path.Join(dir, "Actions.yml"))
	check(err)

	a := actions{}

	check(yaml.Unmarshal([]byte(fileData), &a))

	fileLogger, _ := NewFileLogger(path.Join(dir, a.Logging.File))

	env := NewEnv()
	check(env.LoadFromDir(dir))
	env.LoadFromMap(a.Env)

	return ActionRunner{
		Runner: cr,
		Yaml:   a,
		Env:    env,
		File:   fileLogger,
	}
}

func (a ActionRunner) Run(name string) chan string {
	script := a.Yaml.Actions[name].Script

	a.File.Log("Action: " + name)

	go func() {
		// Receive and print output from the terminal
		for output := range a.Runner.Out {
			// Get the current time
			currentTime := time.Now()

			// Format the time as a string
			timeString := currentTime.Format("2006-01-02 15:04:05")
			timestampedString := fmt.Sprintf("[%s] %s", timeString, output)

			a.File.Log(timestampedString)

			fmt.Println(timestampedString)
		}
	}()

	for _, v := range script {
		a.Runner.In <- a.Env.Replace(v)
	}
	close(a.Runner.In)
	return a.Runner.Out
}

func check(e error) {
	if e != nil {
		fmt.Println(e)
		panic(e)
	}
}

// a := actions.Action("../Test")
// ch := a.Run("install")
// go func() {
// 	for o := range ch {
// 		println(o)
// 	}
// }()
