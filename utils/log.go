package utils

import (
	"io/ioutil"
	"log"
	"os"

	"github.com/sirupsen/logrus"
)

var logger *logrus.Logger

func MustGetLogger() *logrus.Logger {
	return logger
}

func SetLoggerVerbose() {
	logger.SetOutput(os.Stdout)
	logger.Level = logrus.TraceLevel
}

func SetLoggerQuiet() {
	logger.SetOutput(ioutil.Discard)
}

func init() {
	logger = logrus.New()
	logger.Formatter = new(logrus.TextFormatter)
	logger.Level = logrus.WarnLevel
	log.SetOutput(os.Stdout)
}
