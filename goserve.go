package main

import (
	"gopkg.in/v1/yaml"

	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

var cfg ServerConfig

func init() {
	configPath := flag.String("config", "", "Path to configuration")
	checkConfig := flag.Bool("check", false, "Only check config")

	httpEnabled := flag.Bool("http", true, "Enable HTTP listener")
	httpAddr := flag.String("http.addr", ":8080", "HTTP address")
	httpGzip := flag.Bool("http.gzip", true, "Enable gzip compression")
	httpsEnabled := flag.Bool("https", false, "Enable HTTPS listener")
	httpsAddr := flag.String("https.addr", ":8443", "HTTPS address")
	httpsGzip := flag.Bool("https.gzip", true, "Enable gzip compression")
	httpsKey := flag.String("https.key", "", "SSL Key")
	httpsCert := flag.String("https.cert", "", "SSL Cert")

	flag.Parse()

	if *configPath == "" {
		cfg.Listeners = []Listener{}

		if(*httpEnabled) {
			cfg.Listeners = append(cfg.Listeners, Listener{
				Protocol: "http",
				Addr: *httpAddr,
				Gzip: *httpGzip,
			})
		}
		if(*httpsEnabled) {
			cfg.Listeners = append(cfg.Listeners, Listener{
				Protocol: "https",
				Addr: *httpsAddr,
				Gzip: *httpsGzip,
				KeyFile: *httpsKey,
				CertFile: *httpsCert,
			})
		}

		// Serve from first path given on cmdline
		target := flag.Arg(0)
		if target == "" {
			target = "."
		}
		cfg.Serves = []Serve{
			Serve{
				Path:   "/",
				Target: target,
			},
		}
	} else {
		var err error
		cfg, err = readServerConfig(*configPath)
		log.Printf("Reading config: %s\n", *configPath)
		if err != nil {
			log.Fatalln("Couldn't load config:", err)
		}
	}

	cfg.sanitise()

	if !cfg.check() {
		log.Fatalln("Invalid config.")
	}
	if *checkConfig {
		log.Println("Config check passed")
		os.Exit(0)
	}
}

func readServerConfig(filename string) (cfg ServerConfig, err error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}
	err = yaml.Unmarshal(data, &cfg)
	return
}

func main() {
	// Setup handlers
	mux := NewStaticServeMux()
	for _, e := range cfg.Errors {
		mux.HandleError(e.Status, e.handler())
	}
	for _, serve := range cfg.Serves {
		mux.Handle(serve.Path, serve.handler())
	}
	for _, redirect := range cfg.Redirects {
		mux.Handle(redirect.From, redirect.handler())
	}

	// Start listeners
	for _, listener := range cfg.Listeners {
		log.Println("Listen!!!")
		log.Println(listener)

		var h http.Handler = mux
		if len(listener.Headers) > 0 {
			h = CustomHeadersHandler(h, listener.Headers)
		}
		if listener.Gzip {
			h = GzipHandler(h)
		}
		if listener.Protocol == "http" {
			go func() {
				err := http.ListenAndServe(listener.Addr, h)
				if err != nil {
					log.Fatalln(err)
				}
				log.Printf("listening on %s (%s)\n", listener.Addr, listener.Protocol)
			}()
		} else if listener.Protocol == "https" {
			go func() {
				err := http.ListenAndServeTLS(listener.Addr, listener.CertFile, listener.KeyFile, h)
				if err != nil {
					log.Fatalln(err)
				}
				log.Printf("listening on %s (%s)\n", listener.Addr, listener.Protocol)
			}()
		} else {
			log.Printf("Unsupported protocol %s\n", listener.Protocol)
		}
	}

	// Since all the listeners are running in separate gorotines, we have to
	// wait here for a termination signal.
	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt, syscall.SIGTERM)
	<-exit
	os.Exit(0)
}
