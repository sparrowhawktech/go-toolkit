package util

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var defaultLogger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)

type NullLogger log.Logger

func (l *NullLogger) Printf(format string, v ...any) {

}

// Print calls l.Output to print to the logger.
// Arguments are handled in the manner of fmt.Print.
func (l *NullLogger) Print(v ...any) {
}

// Println calls l.Output to print to the logger.
// Arguments are handled in the manner of fmt.Println.
func (l *NullLogger) Println(v ...any) {

}

type LogWriter struct {
	io.Writer
	FileName    string
	MaxSize     int
	MaxFiles    int
	initialized atomic.Bool
	fileNumber  int
	totalBytes  int
	file        *os.File
	mux         *sync.Mutex
}

func (o *LogWriter) Write(p []byte) (n int, err error) {
	o.initialize()
	o.mux.Lock()
	defer o.mux.Unlock()
	w, err := o.file.Write(p)
	if err != nil {
		return w, err
	}
	o.totalBytes += w
	if o.totalBytes >= o.MaxSize {
		o.file.Close()
		o.fileNumber++
		if o.fileNumber >= o.MaxFiles {
			o.fileNumber = 1
		}
		o.createFile()
	}
	return w, nil
}

func (o *LogWriter) initialize() {
	if o.initialized.Load() {
		return
	}
	o.mux.Lock()
	defer o.mux.Unlock()
	if o.initialized.Load() {
		return
	}
	o.initializeFileNumber()
	o.createFile()
	o.initialized.Store(true)
}

func (o *LogWriter) initializeFileNumber() {
	dir := filepath.Dir(o.FileName)
	entries, err := os.ReadDir(dir)
	CheckErr(err)
	o.fileNumber = 1
	var newestFileTime *time.Time
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		name := e.Name()
		lastDotIdx := strings.LastIndex(name, ".")
		if lastDotIdx <= 0 || lastDotIdx+1 == len(name) {
			continue
		}
		if name[:lastDotIdx] != filepath.Base(o.FileName) {
			continue
		}
		info, err := e.Info()
		CheckErr(err)
		if newestFileTime == nil || info.ModTime().After(*newestFileTime) {
			newestFileTime = PTime(info.ModTime())
			number, err := strconv.Atoi(name[lastDotIdx+1:])
			CheckErr(err)
			if o.fileNumber = number + 1; o.fileNumber >= o.MaxFiles {
				o.fileNumber = 1
			}
		}
	}
}

func (o *LogWriter) createFile() {
	name := fmt.Sprintf(o.FileName+".%d", o.fileNumber)
	f, err := os.Create(name)
	CheckErr(err)
	CheckErr(os.Chmod(name, 0644))
	o.file = f
	o.totalBytes = 0
}

type NullWriter struct {
	io.Writer
}

func (o *NullWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

type Loggers struct {
	output     io.Writer
	loggerMap  map[string]*log.Logger
	nullLogger *log.Logger
}

func (o *Loggers) Config(fileName string, maxSize int, maxFiles int, console bool, logFlags int, tags ...string) {
	w := LogWriter{FileName: fileName, MaxFiles: maxFiles, MaxSize: maxSize, mux: &sync.Mutex{}}
	if console {
		o.output = io.MultiWriter(&w, os.Stdout)
		log.SetOutput(o.output)
	} else {
		o.output = &w
		log.SetOutput(&w)
	}
	o.loggerMap = make(map[string]*log.Logger)
	for i := range tags {
		prefix := tags[i]
		o.loggerMap[prefix] = log.New(o.output, prefix+": ", logFlags)
	}
	nullWriter := NullWriter{}
	//nullLogger := log.New(&nullWriter, "", 0)
	nullLogger := log.Logger(NullLogger{})
	nullLogger.SetOutput(&nullWriter)
	nullLogger.SetPrefix("")
	nullLogger.SetFlags(0)
	o.nullLogger = &nullLogger
}

func (o *Loggers) Log(prefix string) *log.Logger {
	if o.loggerMap == nil {
		return defaultLogger
	} else if l, ok := o.loggerMap[prefix]; ok {
		return l
	} else {
		return o.nullLogger
	}
}

var loggers Loggers

func ConfigLoggers(fileName string, maxSize int, maxFiles int, console bool, flags int, tags ...string) {
	loggers.Config(fileName, maxSize, maxFiles, console, flags, tags...)
}

func Log(tag string) *log.Logger {
	return loggers.Log(tag)
}

func Loggable(tag string) bool {
	_, ok := loggers.loggerMap[tag]
	return ok
}
