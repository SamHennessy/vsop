package vsop

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type Proxy struct {
	listener net.Listener
	proxy    *httputil.ReverseProxy
	builder  *Builder
	runner   *Runner
	to       *url.URL
}

func NewProxy(builder *Builder, runner *Runner) *Proxy {
	return &Proxy{
		builder: builder,
		runner:  runner,
	}
}

func (p *Proxy) Run(config *Config, l LineLogNamespace) error {

	// create our reverse proxy
	url, err := url.Parse(config.ProxyTo)
	if err != nil {
		return err
	}
	p.proxy = httputil.NewSingleHostReverseProxy(url)

	r, w := io.Pipe()
	p.proxy.ErrorLog = log.New(w, "", 0)

	go func() {
		for true {
			scanner := bufio.NewScanner(r)
			for scanner.Scan() {
				l.Info(scanner.Text())
			}
			if err := scanner.Err(); err != nil {
				if err != io.EOF {
					l.Err(errors.Wrap(err, "proxy log scanner"))
				}
			}
		}

		l.Debug("Proxy reader done\n")
	}()

	p.to = url

	server := http.Server{Handler: http.HandlerFunc(p.defaultHandler)}

	if config.CertFile != "" && config.KeyFile != "" {
		cer, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
		if err != nil {
			return err
		}

		server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cer}}

		p.listener, err = tls.Listen("tcp", fmt.Sprintf("%s:%d", config.Laddr, config.Port), server.TLSConfig)
		if err != nil {
			return err
		}
	} else {
		p.listener, err = net.Listen("tcp", fmt.Sprintf("%s:%d", config.Laddr, config.Port))
		if err != nil {
			return err
		}
	}

	go server.Serve(p.listener)

	return nil
}

func (p *Proxy) Close() error {
	return p.listener.Close()
}

func (p *Proxy) defaultHandler(res http.ResponseWriter, req *http.Request) {
	errors := p.builder.Errors()
	if len(errors) > 0 {
		res.Write([]byte(errors))
	} else {
		if !p.runner.IsRunning() {
			p.runner.Run()
			// Let the app get going
			p.dialTarget("http://localhost:" + p.to.Port())
		}
		if strings.ToLower(req.Header.Get("Upgrade")) == "websocket" || strings.ToLower(req.Header.Get("Accept")) == "text/event-stream" {
			proxyWebsocket(res, req, p.to)
		} else {
			p.proxy.ServeHTTP(res, req)
		}
	}
}

// TODO: log errors
func (p *Proxy) dialTarget(target string) {
	conn, err := net.DialTimeout("tcp", p.to.Hostname()+":"+p.to.Port(), 5*time.Second)
	if err, ok := err.(*net.OpError); ok && err.Timeout() {
		// fmt.Printf("Timeout error: %s\n", err)
		return
	}
	if err != nil {
		// fmt.Printf("Error: %s\n", err)
		return
	}
	defer conn.Close()
}

func proxyWebsocket(w http.ResponseWriter, r *http.Request, host *url.URL) {
	d, err := net.Dial("tcp", host.Host)
	if err != nil {
		http.Error(w, "Error contacting backend server.", 500)
		fmt.Errorf("Error dialing websocket backend %s: %v", host, err)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Not a hijacker?", 500)
		return
	}
	nc, _, err := hj.Hijack()
	if err != nil {
		fmt.Errorf("Hijack error: %v", err)
		return
	}
	defer nc.Close()
	defer d.Close()

	err = r.Write(d)
	if err != nil {
		fmt.Errorf("Error copying request to target: %v", err)
		return
	}

	errc := make(chan error, 2)
	cp := func(dst io.Writer, src io.Reader) {
		_, err := io.Copy(dst, src)
		errc <- err
	}
	go cp(d, nc)
	go cp(nc, d)
	<-errc
}
