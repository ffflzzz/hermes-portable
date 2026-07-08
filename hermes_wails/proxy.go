package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type proxyState struct {
	mu     sync.Mutex
	server *http.Server
	port   int
	client *http.Client
}

var browserProxy proxyState

func startProxy() int {
	browserProxy.mu.Lock()
	defer browserProxy.mu.Unlock()
	if browserProxy.server != nil {
		return browserProxy.port
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0
	}
	browserProxy.port = l.Addr().(*net.TCPAddr).Port
	browserProxy.client = &http.Client{Timeout: 20}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.Query().Get("url")
		if raw == "" {
			http.Error(w, "url param required", 400)
			return
		}
		dec, err := url.QueryUnescape(raw)
		if err != nil {
			http.Error(w, "bad url", 400)
			return
		}
		req, _ := http.NewRequestWithContext(r.Context(), r.Method, dec, r.Body)
		for _, h := range []string{"Accept", "Content-Type"} {
			if v := r.Header.Get(h); v != "" {
				req.Header.Set(h, v)
			}
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 HermesBrowser/1.0")
		resp, err := browserProxy.client.Do(req)
		if err != nil {
			http.Error(w, fmt.Sprintf("fetch error: %v", err), 502)
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		ct := resp.Header.Get("Content-Type")
		if strings.Contains(ct, "text/html") {
			body = injectBridge(body)
		}
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.Header().Set("X-Proxy-By", "hermes")
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
	})
	browserProxy.server = &http.Server{Handler: mux}
	go browserProxy.server.Serve(l)
	return browserProxy.port
}

func injectBridge(html []byte) []byte {
	s := string(html)
	script := `<script>
(function(){
if(window.__hermesBridge)return;window.__hermesBridge=true;
window.addEventListener("message",function(e){
var d=e.data;
if(!d||!d.__hId)return;
try{
var r;
if(d.__hAct==="eval"){r=String(eval(d.__hJs));}
else if(d.__hAct==="click"){var el=document.querySelector(d.__hSel);if(el){el.click();r="clicked "+d.__hSel}else{r="not found"}}
else if(d.__hAct==="text"){r=document.body?document.body.innerText.slice(0,12000):""}
else if(d.__hAct==="html"){r=document.documentElement.outerHTML.slice(0,12000)}
window.parent.postMessage({__hId:d.__hId,__hRes:r||""},"*")
}catch(ex){window.parent.postMessage({__hId:d.__hId,__hRes:"error: "+ex.message},"*")}
})})();
</script>`
	if i := strings.LastIndex(s, "</head>"); i >= 0 {
		s = s[:i] + script + s[i:]
	} else if i := strings.LastIndex(s, "<body"); i >= 0 {
		s = s[:i] + script + s[i:]
	} else {
		s = script + s
	}
	return []byte(s)
}

func proxyPort() int {
	browserProxy.mu.Lock()
	defer browserProxy.mu.Unlock()
	return browserProxy.port
}
