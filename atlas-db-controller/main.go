package main

import (
	"flag"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	clientset "github.com/infobloxopen/atlas-db/pkg/client/clientset/versioned"
	informers "github.com/infobloxopen/atlas-db/pkg/client/informers/externalversions"
	"github.com/infobloxopen/atlas-db/pkg/signals"
)

var (
	labelselector string
	masterURL     string
	kubeconfig    string
	resyncDur     time.Duration
	LogLevel      string
)

func NewLogger() *logrus.Logger {
	logger := logrus.StandardLogger()

	// Set the log level on the default logger based on command line flag
	logLevels := map[string]logrus.Level{
		"debug":   logrus.DebugLevel,
		"info":    logrus.InfoLevel,
		"warning": logrus.WarnLevel,
		"error":   logrus.ErrorLevel,
		"fatal":   logrus.FatalLevel,
		"panic":   logrus.PanicLevel,
	}
	if level, ok := logLevels[LogLevel]; !ok {
		logger.Errorf("Invalid value %q provided for log level", LogLevel)
		logger.SetLevel(logrus.InfoLevel)
	} else {
		logger.SetLevel(level)
	}

	return logger
}

func main() {
	flag.Parse()
	logger := NewLogger()

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		logger.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	atlasClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building atlas clientset: %s", err.Error())
	}

	filter := func(o *v1.ListOptions) {
		o.LabelSelector = labelselector
	}

	kubeInformerFactory := kubeinformers.NewFilteredSharedInformerFactory(
		kubeClient,
		resyncDur,
		v1.NamespaceAll,
		filter,
	)
	atlasInformerFactory := informers.NewFilteredSharedInformerFactory(
		atlasClient,
		resyncDur,
		v1.NamespaceAll,
		filter,
	)

	controller := NewController(kubeClient, atlasClient, kubeInformerFactory, atlasInformerFactory, logger)

	go kubeInformerFactory.Start(stopCh)
	go atlasInformerFactory.Start(stopCh)

	if err = controller.Run(2, stopCh); err != nil {
		logger.Fatalf("Error running controller: %s", err.Error())
	}
}

func init() {
	flag.DurationVar(&resyncDur, "resync", time.Minute*5, "Resync duration")
	flag.StringVar(&labelselector, "l", "", "Filter all resources by this label selector.")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&LogLevel, "log", "info", "log level")
}
