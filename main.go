package main

import (
	"crypto/tls"
	"flag"
	"github.com/gin-gonic/gin"
	"github.com/mattn/go-xmpp"
	"github.com/mimicloud/easyconfig"
	"log"
	"net/http"
	"os"
	"time"
)

var config = struct {
	Listen         string `json:"listen"`
	JabberUsername string `json:"jabber_username"`
	JabberServer   string `json:"jabber_server"`
	JabberPassword string `json:"jabber_password"`
	JabberDebug    bool   `json:"jabber_debug"`
}{}

var configPath string
var talk *xmpp.Client

const SERVER_INFO = "NotifyServer"

type BasicServerHeader struct {
	gin.ResponseWriter
	ServerInfo string
}

func (w *BasicServerHeader) WriteHeader(code int) {
	if w.Header().Get("Server") == "" {
		w.Header().Add("Server", w.ServerInfo)
	}

	w.ResponseWriter.WriteHeader(code)
}

func ServerHeader() gin.HandlerFunc {
	return func(c *gin.Context) {
		writer := &BasicServerHeader{c.Writer, SERVER_INFO}
		c.Writer = writer
		c.Next()
	}
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	}
}

type Message struct {
	To      string `json:"to"`
	Message string `json:"message"`
}

func SendMessage(client *xmpp.Client, to, msg string) (n int, err error) {
	return client.Send(xmpp.Chat{
		Remote: to,
		Type:   "chat",
		Text:   msg,
	})
}

func init() {
	flag.StringVar(&configPath, "configPath", "config.json", "Path to json file config")
	flag.Parse()
	if flag.NFlag() == 0 {
		flag.PrintDefaults()
		os.Exit(1)
	}

	easyconfig.Parse(configPath, &config)
}

func GetXmppClient() (talk *xmpp.Client, err error) {
	options := xmpp.Options{
		Host:          config.JabberServer,
		User:          config.JabberUsername,
		Password:      config.JabberPassword,
		NoTLS:         true,
		TLSConfig:     &tls.Config{InsecureSkipVerify: true},
		Debug:         config.JabberDebug,
		Session:       true,
		Status:        "online",
		StatusMessage: "online",
	}

	talk, err = options.NewClient()
	go func(client *xmpp.Client) {
		for _ = range time.Tick(3 * time.Second) {
			client.PingC2S(config.JabberUsername, config.JabberServer)
		}
	}(talk)

	return talk, err
}

func main() {
	var err error
	talk, err = GetXmppClient()
	if err != nil {
		log.Fatal(err)
	}

	r := gin.Default()

	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(CORSMiddleware())
	r.Use(ServerHeader())

	r.POST("/", func(c *gin.Context) {
		var m Message
		c.Bind(&m)

		log.Printf("%v", m)
		if m.To != "" && m.Message != "" {
			_, err := SendMessage(talk, m.To, m.Message)
			if err != nil {
				talk, err = GetXmppClient()
				_, errsnd := SendMessage(talk, m.To, m.Message)
				if errsnd != nil {
					log.Printf("%s", err)
					c.JSON(200, gin.H{"status": false})
				}
			} else {
				c.JSON(200, gin.H{"status": true})
			}
		} else {
			c.JSON(200, gin.H{"status": false})
		}
	})

	s := &http.Server{
		Addr:           config.Listen,
		Handler:        r,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	s.ListenAndServe()
}
