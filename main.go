package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/0xAX/notificator"
	"github.com/SamHennessy/vsop/vsop"
	"github.com/fsnotify/fsnotify"
	_ "github.com/joho/godotenv/autoload"
	"github.com/jroimartin/gocui"
	shellwords "github.com/mattn/go-shellwords"
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"
)

var (
	startTime = time.Now()
	// VSOP logger
	logV vsop.LineLogNamespace
	// Space logger
	logS             vsop.LineLogNamespace
	immediate        = false
	buildError       error
	colorGreen       = string([]byte{27, 91, 57, 55, 59, 51, 50, 59, 49, 109})
	colorRed         = string([]byte{27, 91, 57, 55, 59, 51, 49, 59, 49, 109})
	colorReset       = string([]byte{27, 91, 48, 109})
	notifier         = notificator.New(notificator.Options{AppName: "VSOP"})
	notifications    = false
	building         = false
	depRunning       = false
	watcher          *fsnotify.Watcher
	runner           *vsop.Runner
	buildNow         func()
	runNow           func()
	killNow          func()
	depNow           func()
	runPathWatch     func()
	runPathStopWatch func()
	done             chan (bool)
	g                *gocui.Gui
	logTab           = "all"
	findTab          = "match"
)

func main() {
	app := cli.NewApp()
	app.Name = "vsop"
	app.Usage = "A live reload utility for Go web applications."
	app.Action = Run
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "laddr,l",
			Value:  "",
			EnvVar: "VSOP_LADDR",
			Usage:  "listening address for the proxy server",
		},
		cli.IntFlag{
			Name:   "port,p",
			Value:  3000,
			EnvVar: "VSOP_PORT",
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
			Value:  "vsop-bin",
			EnvVar: "VSOP_BIN",
			Usage:  "name of generated binary file",
		},
		cli.StringFlag{
			Name:   "path,t",
			Value:  ".",
			EnvVar: "VSOP_PATH",
			Usage:  "Path to watch files from",
		},
		cli.StringFlag{
			Name:   "build,d",
			Value:  "",
			EnvVar: "VSOP_BUILD",
			Usage:  "Path to build files from (defaults to same value as --path)",
		},
		cli.StringSliceFlag{
			Name:   "excludeDir,x",
			Value:  &cli.StringSlice{},
			EnvVar: "VSOP_EXCLUDE_DIR",
			Usage:  "Relative directories to exclude",
		},
		cli.BoolFlag{
			Name:   "immediate,i",
			EnvVar: "VSOP_IMMEDIATE",
			Usage:  "run the server immediately after it's built",
		},
		cli.BoolFlag{
			Name:   "all",
			EnvVar: "VSOP_ALL",
			Usage:  "reloads whenever any file changes, as opposed to reloading only on .go file change",
		},
		cli.StringFlag{
			Name:   "buildArgs",
			EnvVar: "VSOP_BUILD_ARGS",
			Usage:  "Additional go build arguments",
		},
		cli.StringFlag{
			Name:   "certFile",
			EnvVar: "VSOP_CERT_FILE",
			Usage:  "TLS Certificate",
		},
		cli.StringFlag{
			Name:   "keyFile",
			EnvVar: "VSOP_KEY_FILE",
			Usage:  "TLS Certificate Key",
		},
		cli.BoolFlag{
			Name:   "notifications",
			EnvVar: "VSOP_NOTIFICATIONS",
			Usage:  "Enables desktop notifications",
		},
	}
	app.Commands = []cli.Command{
		{
			Name:      "run",
			ShortName: "r",
			Usage:     "Run the proxy in the current working directory",
			Action:    Run,
		},
	}

	app.Run(os.Args)
}

// Run where all the fun starts
func Run(c *cli.Context) {
	done = make(chan bool)
	laddr := c.GlobalString("laddr")
	port := c.GlobalInt("port")
	appPort := strconv.Itoa(c.GlobalInt("appPort"))
	immediate = c.GlobalBool("immediate")
	keyFile := c.GlobalString("keyFile")
	certFile := c.GlobalString("certFile")
	notifications = c.GlobalBool("notifications")

	go guidash()

	logV = vsop.NewLineLogNamespace("V", nil)
	logS = vsop.NewLineLogNamespace(" ", nil)

	// Set the PORT env
	os.Setenv("PORT", appPort)

	wd, err := os.Getwd()
	if err != nil {
		logV.Fatal(err.Error())
	}

	buildArgs, err := shellwords.Parse(c.GlobalString("buildArgs"))
	if err != nil {
		logV.Fatal(err.Error())
	}

	buildPath := c.GlobalString("build")
	if buildPath == "" {
		buildPath = c.GlobalString("path")
	}
	builder := vsop.NewBuilder(buildPath, c.GlobalString("bin"), c.GlobalBool("godep"), wd, buildArgs)
	runner = vsop.NewRunner(filepath.Join(wd, builder.Binary()), logV, c.Args()...)

	r, w := io.Pipe()
	runner.SetWriter(w)

	go func() {
		ts := false
		fl := vsop.LogInfo
		config := &vsop.LogLineConfig{
			Timestamp:   &ts,
			LevelFilter: &fl,
		}
		appLog := vsop.NewLineLogNamespace("A", config)

		for true {
			scanner := bufio.NewScanner(r)
			for scanner.Scan() {
				appLog.Info(scanner.Text())
			}
			if err := scanner.Err(); err != nil {
				if err != io.EOF {
					logV.Err(errors.Wrap(err, "app stream scanner"))
				}
			}
		}

		logV.Debug("App reader done\n")
	}()

	proxy := vsop.NewProxy(builder, runner)

	config := &vsop.Config{
		Laddr:    laddr,
		Port:     port,
		ProxyTo:  "http://localhost:" + appPort,
		KeyFile:  keyFile,
		CertFile: certFile,
	}

	err = proxy.Run(config, logV)
	if err != nil {
		logV.Fatal(err.Error())
	}

	if laddr != "" {
		logV.Infof("Proxy listening at %s:%d", laddr, port)
	} else {
		logV.Infof("Proxy listening on port %d", port)
	}

	shutdown(runner)

	// build right now
	buildNow = func() {
		building = true
		build(builder, runner, logV)
		building = false
	}

	runNow = func() {
		logV.Info("Run app")
		_, err := runner.Run()
		if err != nil {
			logV.Err(errors.Wrap(err, "Run app"))
		} else {
			logV.Info("App running")
		}
	}
	killNow = func() {
		logV.Info("Killing app")
		runner.Kill()
		logV.Info("Killed app")
	}
	depNow = func() {
		logV.Info("Run dep ensure")
		err := builder.DepEnsure()
		if err != nil {
			logV.Err(err)
		} else {
			logV.Info("Done dep ensure")
		}
	}

	buildNow()

	// Watch sub folders

	// creates a new file watcher
	watcher, err = fsnotify.NewWatcher()
	if err != nil {
		logV.Fatal(errors.Wrap(err, "create new file watcher").Error())
	}
	defer watcher.Close()

	// starting at the root of the project, walk each file/directory searching for
	// directories
	runPathWatch = func() {
		logV.Debugf("Starting watcher, walking all subfolders of %v", c.GlobalString("path"))
		if err := filepath.Walk(c.GlobalString("path"), watchDir); err != nil {
			logV.Err(errors.Wrap(err, "Watcher"))
		} else {
			logV.Debug("Watcher ready")
		}
	}
	runPathStopWatch = func() {
		logV.Debug("Stopping watcher")
		if err := filepath.Walk(c.GlobalString("path"), watchDirStop); err != nil {
			logV.Err(errors.Wrap(err, "Watcher"))
		} else {
			logV.Debug("Watcher stopped")
		}

	}
	go runPathWatch()

	// file watcher
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
						time.Sleep(250 * time.Millisecond)
						buildNow()
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
				logV.Err(err)
			}
		}
	}()

	<-done
}

func build(builder *vsop.Builder, runner *vsop.Runner, logger vsop.LineLogNamespace) {
	logger.Info("Building...")

	buildStart := time.Now()
	err := builder.Build()
	buildTime := time.Now().Sub(buildStart)
	if err != nil {
		buildError = err
		logger.Error("Build failed")
		buildErrors := strings.Split(builder.Errors(), "\n")
		for i := 0; i < len(buildErrors); i++ {
			logger.Error(buildErrors[i])
		}
		if notifications {
			go func() {
				if err := notifier.Push("Build FAILED!", buildErrors[1], "", notificator.UR_CRITICAL); err != nil {
					logger.Err(errors.Wrap(err, "Notification send failed"))
				}
			}()
		}
	} else {
		buildError = nil
		logger.Info("Build finished")
		if immediate {
			runNow()
		}
		if notifications {
			go func() {
				if err := notifier.Push("Built", "Time: "+buildTime.String(), "", notificator.UR_NORMAL); err != nil {
					logger.Err(errors.Wrap(err, "Notification send failed"))
				}
			}()
		}
	}
}

func shutdown(runner *vsop.Runner) {
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

// watchDirStop
func watchDirStop(path string, fi os.FileInfo, err error) error {
	if fi.Mode().IsDir() {
		return watcher.Remove(path)
	}
	return nil
}

type logItem struct {
	message string
	typ     string
}

func guidash() {
	var err error
	g, err = gocui.NewGui(gocui.Output256)
	if err != nil {
		log.Panicln(err)
	}
	defer g.Close()

	g.SetManagerFunc(layout)

	if err := initGlobalKeybindings(g); err != nil {
		log.Panicln(err)
	}

	// Log loop
	go watchLogs()
	// Status
	go func() {
		ticker := time.NewTicker(time.Millisecond * 100)
		for range ticker.C {
			updateStatus()
		}
	}()

	// Blocking
	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		log.Panicln(err)
	}
}

func layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("title", 0, 0, 5, 2); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = false
		fmt.Fprintln(v, "VOSP")
	}

	if v, err := g.SetView("status", 6, 0, 20, 2); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Status"
		fmt.Fprintln(v, "Starting Up")
	}

	if v, err := g.SetView("find", 28, 0, 60, 2); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Editor = gocui.EditorFunc(findEditor)
		v.Editable = true
		v.Title = "[Find] Filter "
	}

	if v, err := g.SetView("logs", -1, 3, maxX, maxY); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Autoscroll = true
		v.Wrap = true
		v.Editor = gocui.EditorFunc(logEditor)
		v.Editable = true
		g.SetCurrentView("logs")
	}

	return nil
}

func initGlobalKeybindings(g *gocui.Gui) error {
	// quit
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}
	return nil
}

func quit(g *gocui.Gui, v *gocui.View) error {
	done <- true
	return gocui.ErrQuit
}

func scrollView(v *gocui.View, dy int) error {
	ox, oy := v.Origin()
	_, maxY := v.Size()
	viewLines := v.ViewBufferLines()
	scrollEnd := len(viewLines) - maxY
	// Stop scrolling too far
	if oy+dy > scrollEnd {
		autoscroll(v)
		return nil
	}

	v.Autoscroll = false
	if err := v.SetOrigin(ox, oy+dy); err != nil {
		return err
	}
	return nil
}

func autoscroll(v *gocui.View) error {
	v.Autoscroll = true
	return nil
}

func watchLogs() {
	lastUpdate := time.Now()
	for {
		logs := vsop.LL().Logs
		if len(logs) > 0 {
			if lastUpdate.Before(logs[len(logs)-1].Timestamp) {
				lastUpdate = logs[len(logs)-1].Timestamp
				renderLogs()
			}
		}
		time.Sleep(time.Millisecond * 100)
	}
}

func renderLogs() {
	logs := vsop.LL().Logs
	g.Update(func(g *gocui.Gui) error {
		v, err := g.View("logs")
		if err != nil {
			logV.Err(errors.Wrap(err, "watch logs getting log view"))
			return err
		}
		v.Clear()
		if logTab == "app" {
			v.Title = " Logs  All [App] VSOP  "
		} else if logTab == "vsop" {
			v.Title = " Logs  All  App [VSOP] "
		} else {
			v.Title = " Logs [All] App  VSOP  "
		}

		for i := 0; i < len(logs); i++ {

			lMsg := logs[i].Message
			findV, err := g.View("find")
			if err != nil {
				// TODO
			}
			find := strings.TrimSpace(findV.Buffer())
			if find != "" {
				pattern := regexp.MustCompile(fmt.Sprintf("(?i)%v", find))
				result := ""
				cur := 0
				for _, submatches := range pattern.FindAllStringSubmatchIndex(lMsg, -1) {
					result += lMsg[cur:submatches[0]]
					result += fmt.Sprintf("\x1b[0;43m%v\x1b[0;39m", lMsg[submatches[0]:submatches[1]])
					cur = submatches[1]
				}
				result += lMsg[cur:]
				if result == lMsg && findTab == "filter" {
					continue
				}
				lMsg = result
			}

			if logs[i].Namespace == "V" && logTab == "app" {
				continue
			}
			if logs[i].Namespace == "A" && logTab == "vsop" {
				continue
			}

			level := "?"
			switch logs[i].Level {
			case vsop.LogDebug:
				level = "D"
			case vsop.LogInfo:
				level = "I"
			case vsop.LogWarn:
				level = "W"
			case vsop.LogError:
				level = "E"
			case vsop.LogFatal:
				level = "F"
			case vsop.LogPanic:
				level = "P"
			}
			fmt.Fprintf(
				v,
				"[%s %s %s] %s\n",
				logs[i].Timestamp.Format("15:04:05"),
				logs[i].Namespace,
				level,
				lMsg,
			)
		}
		return nil
	})
}

func updateStatus() {
	g.Update(func(g *gocui.Gui) error {
		v, err := g.View("status")
		if err != nil {
			logV.Err(errors.Wrap(err, "watch logs getting log view"))
			return err
		}
		v.Clear()
		msg := ""
		v.BgColor = gocui.ColorBlack
		if depRunning {
			msg = "dep Running"
			v.BgColor = gocui.ColorYellow
		} else if building {
			v.BgColor = gocui.ColorYellow
			msg = "Building"
		} else if runner.IsRunning() {
			v.BgColor = gocui.ColorGreen
			msg = "Running"
		} else {
			v.BgColor = gocui.ColorYellow
			msg = "Standby"
		}
		fmt.Fprint(v, msg)

		return nil
	})
}

func findEditor(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	switch {
	case ch != 0 && mod == 0:
		v.EditWrite(ch)
		renderLogs()
	case key == gocui.KeySpace:
		v.EditWrite(' ')
		renderLogs()
	case key == gocui.KeyBackspace || key == gocui.KeyBackspace2:
		v.EditDelete(true)
		renderLogs()
	case key == gocui.KeyDelete:
		v.EditDelete(false)
		renderLogs()
	case key == gocui.KeyInsert:
		v.Overwrite = !v.Overwrite
	case key == gocui.KeyArrowLeft:
		v.MoveCursor(-1, 0, false)
	case key == gocui.KeyArrowRight:
		v.MoveCursor(1, 0, false)
	case key == gocui.KeyCtrlU:
		v.Clear()
		v.SetCursor(0, 0)
		g.Cursor = false
		renderLogs()
	case key == gocui.KeyEnter:
		g.Cursor = false
		g.SetCurrentView("logs")
		renderLogs()
	case key == gocui.KeyTab:
		if findTab == "match" {
			findTab = "filter"
			v.Title = " Find [Filter]"
		} else {
			findTab = "match"
			v.Title = "[Find] Filter "
		}
		renderLogs()
	}
}
func logEditor(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	switch {
	case key == gocui.KeyEnter:
		logS.Info("")
	case key == gocui.KeyArrowDown:
		scrollView(v, 1)
	case key == gocui.KeyArrowUp:
		scrollView(v, -1)
	case key == gocui.KeyEnd: // autoscroll
		autoscroll(v)
	case key == gocui.KeyCtrlB: // build
		killNow()
		buildNow()
	case key == gocui.KeyCtrlD: // dep ensure
		depRunning = true
		runPathStopWatch()
		depNow()
		runPathWatch()
		buildNow()
		depRunning = false
	case key == gocui.KeyCtrlR: // run/restart app
		killNow()
		runNow()
	case key == gocui.KeyCtrlK: // kill/stop app
		killNow()
	case key == gocui.KeyTab:
		if logTab == "all" {
			logTab = "app"
		} else if logTab == "app" {
			logTab = "vsop"
		} else if logTab == "vsop" {
			logTab = "all"
		}
		renderLogs()
	case key == gocui.KeyCtrlF:
		g.Cursor = true
		g.SetCurrentView("find")
	}
}
