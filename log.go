package logging

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

//log level
const (
	CRITICAL = iota
	ERROR
	WARNING
	NOTICE
	INFO
	DEBUG
	TRACE
)

//log writer
const (
	CONSOLELOG = "console" //consolewriter
	FILELOG    = "file"    //filewriter
)

//log info
type Message struct {
	level int
	msg   string
	time  time.Time
}

// message pool
var logMsgPool = &sync.Pool{
	New: func() interface{} {
		return &Message{}
	},
}

type nameWriter struct {
	logWriter
	name string
}

type logWriter interface {
	Init(config string) error
	WriteMsg(time time.Time, msg string, level int) error
	Destroy()
	Flush()
}

//logWriter creater
type newlogWriterFunc func() logWriter

//all logWriter provider in the map
//key is provider name
//value is a func that create a logWriter
var adapters = make(map[string]newlogWriterFunc)
var levelPrefix = [6]string{"CRITICAL ", "ERROR ", "WARNING ", "NOTICE ", "INFO ", "DEBUG "}

//register a log provier by the provided name
func Register(name string, log newlogWriterFunc) {
	if log == nil {
		panic("Register log provider is nil")
	}
	if _, ok := adapters[name]; ok {
		panic("Register called twice for common log provider " + name)
	}
	adapters[name] = log
}

const defaultAsyncMsgLen = 1e3

type Logger struct {
	level       int
	init        bool
	enableDepth bool
	logDepth    int
	async       bool
	msgChan     chan *Message
	msgChanLen  int64
	signalChan  chan string
	lock        sync.Mutex
	wg          sync.WaitGroup
	writers     []*nameWriter
}

func NewLogger(channeLens ...int64) *Logger {
	log := new(Logger)
	log.level = DEBUG
	log.logDepth = 2
	log.msgChanLen = append(channeLens, 0)[0]
	if log.msgChanLen <= 0 {
		log.msgChanLen = defaultAsyncMsgLen
	}
	log.signalChan = make(chan string, 1)
	return log
}

// SetLogger provides a given logger adapter into Logger with config string.
// config need to be correct JSON as string: {"interval":360}.
func (l *Logger) setLogger(writerName string, configs ...string) error {
	config := append(configs, "{}")[0]
	for _, w := range l.writers {
		if w.name == writerName {
			return fmt.Errorf("logs: duplicate writer %q (you have set this logger before)", writerName)
		}
	}

	logWriterFunc, ok := adapters[writerName]
	if !ok {
		return fmt.Errorf("logs: unknown writer %q (forgotten Register?)", writerName)
	}

	lw := logWriterFunc()
	err := lw.Init(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "logs.Logger.SetLogger: "+err.Error())
		return err
	}
	l.writers = append(l.writers, &nameWriter{name: writerName, logWriter: lw})
	return nil
}

func (l *Logger) SetLogger(writerName string, configs ...string) error {
	l.lock.Lock()
	defer l.lock.Unlock()
	if !l.init {
		l.writers = []*nameWriter{}
		l.init = true
	}
	return l.setLogger(writerName, configs...)
}

//send log msg to writer and writer to record log
func (l *Logger) sendMsgToWriter(time time.Time, msg string, level int) {
	//send msg to all writer
	for _, w := range l.writers {
		err := w.WriteMsg(time, msg, level)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to WriteMsg to writer:%v,error:%v\n", w.name, err)
		}
	}
}

func (l *Logger) writeMsg(level int, msg string, v ...interface{}) error {
	if len(v) > 0 {
		msg = fmt.Sprintf(msg, v...)
	}
	t := time.Now()
	if l.enableDepth {
		_, file, line, ok := runtime.Caller(l.logDepth)
		if !ok {
			file = "???"
			line = 0
		}
		_, filename := path.Split(file)
		//		strings.Replace(file,)
		//		filename := filepath.Ext(file)
		msg = "[" + filename + strconv.Itoa(line) + "] " + msg
	}

	msg = levelPrefix[level] + msg

	if l.async {
		message := logMsgPool.Get().(*Message)
		message.level = level
		message.msg = msg
		message.time = t
		l.msgChan <- message
	} else {
		l.sendMsgToWriter(t, msg, level)
	}

	return nil
}

func (l *Logger) SetLevel(lv int) {
	l.level = lv
}

func (l *Logger) SetLogDepth(d int) {
	l.logDepth = d
}

func (l *Logger) GetLogDepth() int {
	return l.logDepth
}

func (l *Logger) EnableDepth(b bool) {
	l.enableDepth = b
}

func (l *Logger) pullMsgFromChan() {
	shutDown := false
	for {
		select {
		case msg := <-l.msgChan:
			l.sendMsgToWriter(msg.time, msg.msg, msg.level)
			logMsgPool.Put(msg)
		case signal := <-l.signalChan:
			l.Flush()
			if signal == "close" {
				for _, w := range l.writers {
					w.Destroy()
				}
				l.writers = nil
				shutDown = true
			}

			l.wg.Done()
		}

		if shutDown {
			break
		}
	}
}

//Critical Log level message
func (l *Logger) Critical(format string, v ...interface{}) {
	if CRITICAL > l.level {
		return
	}

	l.writeMsg(CRITICAL, format, v...)
}

// Error Log ERROR level message.
func (l *Logger) Error(format string, v ...interface{}) {
	if ERROR > l.level {
		return
	}
	l.writeMsg(ERROR, format, v...)
}

// Warning Log WARNING level message.
func (l *Logger) Warning(format string, v ...interface{}) {
	if WARNING > l.level {
		return
	}
	l.writeMsg(WARNING, format, v...)
}

// Notice Log NOTICE level message.
func (l *Logger) Notice(format string, v ...interface{}) {
	if NOTICE > l.level {
		return
	}
	l.writeMsg(NOTICE, format, v...)
}

// Informational Log INFORMATIONAL level message.
func (l *Logger) Info(format string, v ...interface{}) {
	if INFO > l.level {
		return
	}
	l.writeMsg(INFO, format, v...)
}

// Debug Log DEBUG level message.
func (l *Logger) Debug(format string, v ...interface{}) {
	if DEBUG > l.level {
		return
	}
	l.writeMsg(DEBUG, format, v...)
}

// Async set the log to async and start the goroutine
func (l *Logger) Async(msgLen ...int64) *Logger {
	l.lock.Lock()
	defer l.lock.Unlock()
	if l.async {
		return l
	}
	l.async = true
	if len(msgLen) > 0 && msgLen[0] > 0 {
		l.msgChanLen = msgLen[0]
	}
	l.msgChan = make(chan *Message, l.msgChanLen)
	logMsgPool = &sync.Pool{
		New: func() interface{} {
			return &Message{}
		},
	}
	l.wg.Add(1)
	go l.pullMsgFromChan()
	return l
}

func (l *Logger) Flush() {
	if l.async {
		for {
			if len(l.msgChan) > 0 {
				msg := <-l.msgChan
				l.sendMsgToWriter(msg.time, msg.msg, msg.level)
				logMsgPool.Put(msg)
				continue
			}
			break
		}
	}

	for _, w := range l.writers {
		w.Flush()
	}
}

// Close close logger, flush all chan data and destroy all adapters in Logger.
func (l *Logger) Close() {
	if l.async {
		l.signalChan <- "close"
		l.wg.Wait()
		close(l.msgChan)
	} else {
		l.Flush()
		for _, l := range l.writers {
			l.Destroy()
		}
		l.writers = nil
	}
	close(l.signalChan)
}

/****
*this logger reference be used for application everywhere
*
***/
var logger = NewLogger()

func GetLogger() *Logger {
	return logger
}

func Async(msgLen ...int64) *Logger {
	return logger.Async(msgLen...)
}

func SetLevel(lv int) {
	logger.SetLevel(lv)
}

func EnableDepth(b bool) {
	logger.EnableDepth(b)
}

func SetLogDepth(d int) {
	logger.SetLogDepth(d)
}

func SetLogger(writer string, config ...string) error {
	return logger.SetLogger(writer, config...)
}

func formatLog(f interface{}, v ...interface{}) string {
	var msg string
	switch f.(type) {
	case string:
		msg = f.(string)
		if len(v) == 0 {
			return msg
		}
		if strings.Contains(msg, "%") && !strings.Contains(msg, "%%") {
			//format string
		} else {
			//do not contain format char
			msg += strings.Repeat(" %v", len(v))
		}
	default:
		msg = fmt.Sprint(f)
		if len(v) == 0 {
			return msg
		}
		msg += strings.Repeat(" %v", len(v))
	}
	return fmt.Sprintf(msg, v...)
}

// Critical logs a message at critical level.
func Critical(f interface{}, v ...interface{}) {
	logger.Critical(formatLog(f, v...))
}

// Error logs a message at error level.
func Error(f interface{}, v ...interface{}) {
	logger.Error(formatLog(f, v...))
}

// Warning logs a message at warning level.
func Warning(f interface{}, v ...interface{}) {
	logger.Warning(formatLog(f, v...))
}

// Notice logs a message at notice level.
func Notice(f interface{}, v ...interface{}) {
	logger.Notice(formatLog(f, v...))
}

// Informational logs a message at info level.
func Info(f interface{}, v ...interface{}) {
	logger.Info(formatLog(f, v...))
}

// Debug logs a message at debug level.
func Debug(f interface{}, v ...interface{}) {
	logger.Debug(formatLog(f, v...))
}
