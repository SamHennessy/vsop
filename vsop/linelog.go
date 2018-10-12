package vsop

import (
	"fmt"
	"os"
	"sync"
	"time"
)

type LineLog struct {
	Config          LogLineConfig
	ConfigNS        map[string]*LogLineConfig
	Cap             int
	Logs            []LogLineLog
	NamespaceFilter string
	RegexFilter     string
}

type LogLineLog struct {
	Message   string
	Namespace string
	Level     LogLineLevel
	Timestamp time.Time
}

type LogLineConfig struct {
	LevelFilter *LogLineLevel
	Timestamp   *bool
}

type LogLineLevel int

const (
	LogDebug LogLineLevel = iota
	LogInfo
	LogWarn
	LogError
	LogFatal
	LogPanic
)

var ll *LineLog
var llOnce sync.Once

func LL() *LineLog {
	llOnce.Do(func() {
		// Global defaults
		tru := true
		level := LogDebug
		ll = &LineLog{
			Cap: 1000, // TODO: implement? maybe with a really big default
			Config: LogLineConfig{
				LevelFilter: &level,
				Timestamp:   &tru,
			},
			ConfigNS: make(map[string]*LogLineConfig),
		}
	})
	return ll
}

func NewLineLogNamespace(namespace string, config *LogLineConfig) LineLogNamespace {

	LL().ConfigNS[namespace] = config
	return LineLogNamespace{
		Namespace: namespace,
		Log:       LL(),
	}
}

func (l *LineLog) Debug(namespace string, msg string) {
	l.Logs = append(l.Logs, LogLineLog{Namespace: namespace, Timestamp: time.Now(), Message: msg, Level: LogDebug})
}

func (l *LineLog) Info(namespace string, msg string) {
	l.Logs = append(l.Logs, LogLineLog{Namespace: namespace, Timestamp: time.Now(), Message: msg, Level: LogInfo})
}

func (l *LineLog) Warn(namespace string, msg string) {
	l.Logs = append(l.Logs, LogLineLog{Namespace: namespace, Timestamp: time.Now(), Message: msg, Level: LogWarn})
}

func (l *LineLog) Error(namespace string, msg string) {
	l.Logs = append(l.Logs, LogLineLog{Namespace: namespace, Timestamp: time.Now(), Message: msg, Level: LogError})
}

func (l *LineLog) Fatal(namespace string, msg string) {
	l.Logs = append(l.Logs, LogLineLog{Namespace: namespace, Timestamp: time.Now(), Message: msg, Level: LogFatal})
	os.Exit(-1)
}

func (l *LineLog) Panic(namespace string, msg string) {
	l.Logs = append(l.Logs, LogLineLog{Namespace: namespace, Timestamp: time.Now(), Message: msg, Level: LogPanic})
	panic(msg)
}

func (l *LineLog) Err(namespace string, err error) {
	l.Logs = append(l.Logs, LogLineLog{Namespace: namespace, Timestamp: time.Now(), Message: err.Error(), Level: LogError})
}

type LineLogNamespace struct {
	Namespace string
	Log       *LineLog
}

func (n *LineLogNamespace) Debug(msg string) {
	n.Log.Debug(n.Namespace, msg)
}
func (n *LineLogNamespace) Debugf(format string, args ...interface{}) {
	n.Debug(fmt.Sprintf(format, args...))
}

func (n *LineLogNamespace) Info(msg string) {
	n.Log.Info(n.Namespace, msg)
}
func (n *LineLogNamespace) Infof(format string, a ...interface{}) {
	n.Info(fmt.Sprintf(format, a...))
}

func (n *LineLogNamespace) Warn(msg string) {
	n.Log.Warn(n.Namespace, msg)
}

func (n *LineLogNamespace) Error(msg string) {
	n.Log.Error(n.Namespace, msg)
}

func (n *LineLogNamespace) Fatal(msg string) {
	n.Log.Fatal(n.Namespace, msg)
}

func (n *LineLogNamespace) Panic(msg string) {
	n.Log.Panic(n.Namespace, msg)
}

func (n *LineLogNamespace) Err(err error) {
	n.Log.Error(n.Namespace, err.Error())
}
