package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/0xAX/notificator"
	"github.com/codegangsta/envy/lib"
	"github.com/codegangsta/gin/lib"
	"github.com/fsnotify/fsnotify"
	shellwords "github.com/mattn/go-shellwords"
	"gopkg.in/urfave/cli.v1"
)

var watcher *fsnotify.Watcher

var (
	startTime     = time.Now()
	logger        = log.New(os.Stdout, "[vsop] ", 0)
	immediate     = false
	buildError    error
	colorGreen    = string([]byte{27, 91, 57, 55, 59, 51, 50, 59, 49, 109})
	colorRed      = string([]byte{27, 91, 57, 55, 59, 51, 49, 59, 49, 109})
	colorReset    = string([]byte{27, 91, 48, 109})
	notifier      = notificator.New(notificator.Options{AppName: "VSOP Build"})
	notifications = false
)

func main() {
	app := cli.NewApp()
	app.Name = "vsop"
	app.Usage = "A live reload utility for Go web applications."
	app.Action = MainAction
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "laddr,l",
			Value:  "",
			EnvVar: "GIN_LADDR",
			Usage:  "listening address for the proxy server",
		},
		cli.IntFlag{
			Name:   "port,p",
			Value:  3000,
			EnvVar: "GIN_PORT",
			Usage:  "port for the proxy server",
		},
		cli.IntFlag{
			Name:   "appPort,a",
			Value:  3001,
			EnvVar: "BIN_APP_PORT",
			Usage:  "port for the Go web server",
		},
		cli.StringFlag{
			Name:   "bin,b",
			Value:  "gin-bin",
			EnvVar: "GIN_BIN",
			Usage:  "name of generated binary file",
		},
		cli.StringFlag{
			Name:   "path,t",
			Value:  ".",
			EnvVar: "GIN_PATH",
			Usage:  "Path to watch files from",
		},
		cli.StringFlag{
			Name:   "build,d",
			Value:  "",
			EnvVar: "GIN_BUILD",
			Usage:  "Path to build files from (defaults to same value as --path)",
		},
		cli.StringSliceFlag{
			Name:   "excludeDir,x",
			Value:  &cli.StringSlice{},
			EnvVar: "GIN_EXCLUDE_DIR",
			Usage:  "Relative directories to exclude",
		},
		cli.BoolFlag{
			Name:   "immediate,i",
			EnvVar: "GIN_IMMEDIATE",
			Usage:  "run the server immediately after it's built",
		},
		cli.BoolFlag{
			Name:   "all",
			EnvVar: "GIN_ALL",
			Usage:  "reloads whenever any file changes, as opposed to reloading only on .go file change",
		},
		cli.BoolFlag{
			Name:   "godep,g",
			EnvVar: "GIN_GODEP",
			Usage:  "use godep when building",
		},
		cli.StringFlag{
			Name:   "buildArgs",
			EnvVar: "GIN_BUILD_ARGS",
			Usage:  "Additional go build arguments",
		},
		cli.StringFlag{
			Name:   "certFile",
			EnvVar: "GIN_CERT_FILE",
			Usage:  "TLS Certificate",
		},
		cli.StringFlag{
			Name:   "keyFile",
			EnvVar: "GIN_KEY_FILE",
			Usage:  "TLS Certificate Key",
		},
		cli.StringFlag{
			Name:   "logPrefix",
			EnvVar: "GIN_LOG_PREFIX",
			Usage:  "Log prefix",
			Value:  "vsop",
		},
		cli.BoolFlag{
			Name:   "notifications",
			EnvVar: "GIN_NOTIFICATIONS",
			Usage:  "Enables desktop notifications",
		},
	}
	app.Commands = []cli.Command{
		{
			Name:      "run",
			ShortName: "r",
			Usage:     "Run the gin proxy in the current working directory",
			Action:    MainAction,
		},
		{
			Name:      "env",
			ShortName: "e",
			Usage:     "Display environment variables set by the .env file",
			Action:    EnvAction,
		},
	}

	app.Run(os.Args)
}

func MainAction(c *cli.Context) {
	laddr := c.GlobalString("laddr")
	port := c.GlobalInt("port")
	// all := c.GlobalBool("all")
	appPort := strconv.Itoa(c.GlobalInt("appPort"))
	immediate = c.GlobalBool("immediate")
	keyFile := c.GlobalString("keyFile")
	certFile := c.GlobalString("certFile")
	logPrefix := c.GlobalString("logPrefix")
	notifications = c.GlobalBool("notifications")

	logger.SetPrefix(fmt.Sprintf("[%s] ", logPrefix))

	// Bootstrap the environment
	envy.Bootstrap()

	// Set the PORT env
	os.Setenv("PORT", appPort)

	wd, err := os.Getwd()
	if err != nil {
		logger.Fatal(err)
	}

	buildArgs, err := shellwords.Parse(c.GlobalString("buildArgs"))
	if err != nil {
		logger.Fatal(err)
	}

	buildPath := c.GlobalString("build")
	if buildPath == "" {
		buildPath = c.GlobalString("path")
	}
	builder := gin.NewBuilder(buildPath, c.GlobalString("bin"), c.GlobalBool("godep"), wd, buildArgs)
	runner := gin.NewRunner(filepath.Join(wd, builder.Binary()), c.Args()...)
	runner.SetWriter(os.Stdout)
	proxy := gin.NewProxy(builder, runner)

	config := &gin.Config{
		Laddr:    laddr,
		Port:     port,
		ProxyTo:  "http://localhost:" + appPort,
		KeyFile:  keyFile,
		CertFile: certFile,
	}

	err = proxy.Run(config)
	if err != nil {
		logger.Fatal(err)
	}

	if laddr != "" {
		logger.Printf("Listening at %s:%d\n", laddr, port)
	} else {
		logger.Printf("Listening on port %d\n", port)
	}

	shutdown(runner)

	// build right now
	build(builder, runner, logger)

	// Watch sub folders

	// creates a new file watcher
	// TODO: check error
	watcher, _ = fsnotify.NewWatcher()
	defer watcher.Close()
	//

	// starting at the root of the project, walk each file/directory searching for
	// directories
	if err := filepath.Walk(c.GlobalString("path"), watchDir); err != nil {
		logger.Println("ERROR", err)
	}

	done := make(chan bool)

	//
	go func() {
		lastBuild := time.Now()
		for {
			select {
			// watch for events
			case event := <-watcher.Events:
				if event.Op == fsnotify.Write && strings.HasSuffix(event.Name, ".go") {
					td := time.Now().Sub(lastBuild)
					if td.Seconds() > 1 {
						runner.Kill()
						// Wait for any post save hooks to run
						time.Sleep(100 * time.Millisecond)
						build(builder, runner, logger)
						lastBuild = time.Now()
					}
				} else if event.Op == fsnotify.Create {
					info, err := os.Stat(event.Name)
					if err == nil && info.IsDir() {
						watcher.Add(event.Name)
					}
				}

				// watch for errors
			case err := <-watcher.Errors:
				logger.Printf("ERROR %v\n", err)
			}
		}
	}()

	<-done
}

func EnvAction(c *cli.Context) {
	logPrefix := c.GlobalString("logPrefix")
	logger.SetPrefix(fmt.Sprintf("[%s] ", logPrefix))

	// Bootstrap the environment
	env, err := envy.Bootstrap()
	if err != nil {
		logger.Fatalln(err)
	}

	for k, v := range env {
		fmt.Printf("%s: %s\n", k, v)
	}

}

func build(builder gin.Builder, runner gin.Runner, logger *log.Logger) {
	logger.Println("Building...")

	buildStart := time.Now()
	err := builder.Build()
	buildTime := time.Now().Sub(buildStart)
	if err != nil {
		buildError = err
		logger.Printf("%sBuild failed%s\n", colorRed, colorReset)
		fmt.Println(builder.Errors())
		buildErrors := strings.Split(builder.Errors(), "\n")
		if notifications {
			go func() {
				if err := notifier.Push("Build FAILED!", buildErrors[1], "", notificator.UR_CRITICAL); err != nil {
					logger.Println("Notification send failed")
				}
			}()
		}
	} else {
		buildError = nil
		logger.Printf("%sBuild finished%s\n", colorGreen, colorReset)
		if immediate {
			runner.Run()
		}
		if notifications {
			go func() {

				if err := notifier.Push("Built", "Time: "+buildTime.String(), "", notificator.UR_NORMAL); err != nil {
					logger.Println("Notification send failed")
				}
			}()
		}
	}
}

func shutdown(runner gin.Runner) {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		s := <-c
		log.Println("Got signal: ", s)
		err := runner.Kill()
		if err != nil {
			log.Print("Error killing: ", err)
		}
		os.Exit(1)
	}()
}

// watchDir gets run as a walk func, searching for directories to add watchers to
func watchDir(path string, fi os.FileInfo, err error) error {

	// since fsnotify can watch all the files in a directory, watchers only need
	// to be added to each nested directory
	if fi.Mode().IsDir() {
		return watcher.Add(path)
	}

	return nil
}
