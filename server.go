package main

import (
	"context"
	"crypto/tls"
	"flag"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/carlmjohnson/heffalump/heff"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/sandalwing/echo-logrusmiddleware"
)

var (
	dir      string
	host     string
	port     string
	proxy    string
	loglevel string
	logfile  string
	certfile string
	keyfile  string
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
	log.SetFormatter(&log.JSONFormatter{})
	flag.StringVar(&dir, "dir", ".", "the directory to serve files from.")
	flag.StringVar(&host, "host", "localhost", "the host to listen with/on")
	flag.StringVar(&port, "port", "8888", "the port to listen on")
	flag.StringVar(&proxy, "proxy", "", "A http proxy to use for output traffic")
	flag.StringVar(&loglevel, "loglevel", "info", "Level of debugging {debug|info|warn|error|panic}")
	flag.StringVar(&logfile, "logfile", "/var/log/ttfn.log", "Location of log file")
	flag.StringVar(&certfile, "cert", "", "certificate file")
	flag.StringVar(&keyfile, "key", "", "private key file")
}

func randInt(min int, max int) int {
	return min + rand.Intn(max-min)
}
func randColor() color.Color {
	return color.RGBA{uint8(randInt(0, 255)), uint8(randInt(0, 255)), uint8(randInt(0, 255)), 0xFF}
}

func serveImage(basepath string, sepecifcpath string, w *echo.Response) {
	width := randInt(10, 50)
	height := width

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, randColor())
		}
	}
	jpeg.Encode(w.Writer, img, nil)
}

func customHTTPErrorHandler(err error, c echo.Context) {
	req := c.Request()
	resp := c.Response()
	if strings.HasSuffix(req.URL.Path, ".jpg") {
		serveImage(dir, req.URL.Path, resp)
	} else {
		heff.DefaultHoneypot(resp, req)
	}
}

func main() {

	log.SetFormatter(&log.JSONFormatter{})
	flag.Parse()

	logL, err := log.ParseLevel(loglevel)
	if err != nil {
		log.Warn("Unable to parse loglevel, setting to default: info")
		log.SetLevel(log.InfoLevel)
	} else {
		log.SetLevel(logL)
	}
	file, err := os.OpenFile(logfile, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.SetOutput(os.Stdout)
		log.Warn("Error opening logfile: " + err.Error())
	} else {
		log.SetOutput(file)
	}
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = customHTTPErrorHandler
	e.Logger = logrusmiddleware.Logger{log.StandardLogger()}
	e.Use(logrusmiddleware.Hook())
	e.Use(middleware.RequestID())
	e.Use(middleware.Secure())

	e.Static("/", dir)

	config := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
	}

	var srv *http.Server
	if certfile == "" || keyfile == "" {
		srv = &http.Server{
			Addr:         host + ":" + port,
			WriteTimeout: 10 * time.Minute,
			ReadTimeout:  5 * time.Minute,
		}
	} else {
		srv = &http.Server{
			Addr:         host + ":" + port,
			WriteTimeout: 10 * time.Minute,
			ReadTimeout:  5 * time.Minute,
			TLSConfig:    config,
			TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
		}
	}

	// Start server
	go func() {
		if err := e.StartServer(srv); err != nil {
			log.Error(err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 10 seconds.
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
}
