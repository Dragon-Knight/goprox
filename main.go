package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/net/proxy"
)

func main() {

	log.Println("Simple HTTP to HTTPS proxy server")

	cfg_port := flag.Int("port", 8888, "server port")
	//cfg_socks := flag.String("socks", "", "socks server, format: host:port")

	flag.Parse()

	//if *cfg_socks == "" || *cfg_port == 0 {
	//	fmt.Println("Error: missing required parameters")
	//	flag.Usage()
	//	os.Exit(1)
	//}

	server := &http.Server{
		Addr:    ":" + strconv.Itoa(*cfg_port),
		Handler: http.HandlerFunc(proxyHandler),
	}

	ip := getLocalIP()
	port := strconv.Itoa(*cfg_port)
	log.Printf("Server started at %s:%s\n", ip, port)

	log.Fatal(server.ListenAndServe())
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request: %s %s %s", r.Method, r.Host, r.URL.RequestURI())

	// SOCKS5 dialer
	socksDialer, err := proxy.SOCKS5("tcp", "10.0.0.101:1080", nil, proxy.Direct)
	if err != nil {
		log.Printf("SOCKS5 error: %v", err)
		http.Error(w, "SOCKS5 error", http.StatusInternalServerError)
		return
	}

	// Чистый хост без порта
	host := r.Host
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}

	// Транспорт через SOCKS
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			log.Println("SOCKS DIAL:", host+":443")
			return socksDialer.Dial(network, host+":443")
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   60 * 1e9,
	}

	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""
	outReq.URL.Scheme = "https"
	outReq.URL.Host = host + ":443"
	outReq.Host = host

	outReq.Header.Del("Proxy-Connection")
	outReq.Header.Del("Proxy-Authorization")

	// Браузерный User-Agent
	if outReq.Header.Get("User-Agent") == "" {
		outReq.Header.Set("User-Agent",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) "+
				"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	}

	log.Println("Outgoing URL:", outReq.URL.String())

	resp, err := client.Do(outReq)
	if err != nil {
		log.Printf("Error sending outbound request: %v", err)
		http.Error(w, "Upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	log.Printf("Received response %s", resp.Status)

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func getLocalIP() string {
	conn, _ := net.Dial("udp", "8.8.8.8:80")
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP.String()
}
