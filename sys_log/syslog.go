package syslog

import (
	"os"
	"log"
	"io"
	"log/syslog"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"fmt"
)

type Logger struct {
	debug *log.Logger
	info  *log.Logger
	warn  *log.Logger
	error *log.Logger
	fatal *log.Logger
	aws   *log.Logger
}

func New(address, tag string) (*Logger, error) {
	var multiWriter io.Writer = os.Stdout
	if address != "" {
		sysLogWriter, err := syslog.Dial("udp", address, syslog.LOG_EMERG|syslog.LOG_KERN, tag)
		if err != nil {
			return nil, err
		}
		multiWriter = io.MultiWriter(sysLogWriter, os.Stdout)
	}
	l := Logger{}
	//todo:uncomment
	//l.debug = log.New(os.Stdout, "DEBUG ", log.Ldate|log.Lmicroseconds|log.LUTC)
	l.debug = log.New(multiWriter, "DEBUG ", log.Ldate|log.Lmicroseconds|log.LUTC)
	l.info = log.New(multiWriter, "INFO ", log.Ldate|log.Lmicroseconds|log.LUTC)
	l.warn = log.New(multiWriter, "WARNING ", log.Ldate|log.Lmicroseconds|log.LUTC)
	l.error = log.New(multiWriter, "ERROR ", log.Ldate|log.Lmicroseconds|log.LUTC)
	l.fatal = log.New(multiWriter, "FATAL ", log.Ldate|log.Lmicroseconds|log.LUTC)
	l.aws = log.New(multiWriter, "AWS SDK ", log.Ldate|log.Lmicroseconds|log.LUTC)
	return &l, nil
}

func (l *Logger) Debugf(ctx *lambdacontext.LambdaContext, s string, args ...interface{}) {
	if ctx != nil {
		s = fmt.Sprintf("[%s] %s", ctx.AwsRequestID, s)
	}
	l.debug.Printf(s, args...)
}

func (l *Logger) Debugln(ctx *lambdacontext.LambdaContext, s string) {
	if ctx != nil {
		s = fmt.Sprintf("[%s] %s", ctx.AwsRequestID, s)
	}
	l.debug.Println(s)
}

func (l *Logger) Infof(ctx *lambdacontext.LambdaContext, s string, args ...interface{}) {
	if ctx != nil {
		s = fmt.Sprintf("[%s] %s", ctx.AwsRequestID, s)
	}
	l.info.Printf(s, args...)
}

func (l *Logger) Infoln(ctx *lambdacontext.LambdaContext, s string) {
	if ctx != nil {
		s = fmt.Sprintf("[%s] %s", ctx.AwsRequestID, s)
	}
	l.info.Println(s)
}

func (l *Logger) Warnf(ctx *lambdacontext.LambdaContext, s string, args ...interface{}) {
	if ctx != nil {
		s = fmt.Sprintf("[%s] %s", ctx.AwsRequestID, s)
	}
	l.warn.Printf(s, args...)
}

func (l *Logger) Warnln(ctx *lambdacontext.LambdaContext, s string) {
	if ctx != nil {
		s = fmt.Sprintf("[%s] %s", ctx.AwsRequestID, s)
	}
	l.warn.Println(s)
}

func (l *Logger) Errorf(ctx *lambdacontext.LambdaContext, s string, args ...interface{}) {
	if ctx != nil {
		s = fmt.Sprintf("[%s] %s", ctx.AwsRequestID, s)
	}
	l.error.Printf(s, args...)
}

func (l *Logger) Errorln(ctx *lambdacontext.LambdaContext, s string) {
	if ctx != nil {
		s = fmt.Sprintf("[%s] %s", ctx.AwsRequestID, s)
	}
	l.error.Printf(s)
}

func (l *Logger) Fatalf(ctx *lambdacontext.LambdaContext, s string, args ...interface{}) {
	if ctx != nil {
		s = fmt.Sprintf("[%s] %s", ctx.AwsRequestID, s)
	}
	l.fatal.Printf(s, args...)
	os.Exit(1)
}

func (l *Logger) Fatalln(ctx *lambdacontext.LambdaContext, s string) {
	if ctx != nil {
		s = fmt.Sprintf("[%s] %s", ctx.AwsRequestID, s)
	}
	l.fatal.Println(s)
	os.Exit(1)
}

func (l *Logger) AwsLog(args ...interface{}) {
	l.aws.Println(args...)
}
