package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/gorilla/websocket"
	"github.com/spf13/viper"
)

const (
	// VERSION is the version string for Cowherd
	VERSION = "1.2"
	// AUTHOR is me.
	AUTHOR = "Camron G. Levanger <camronlevanger@gmail.com>"
	// USAGE is for terminal help.
	USAGE = `
Example:
    cowherd <env> my-server-1
    cowherd <env> "my-server*"  (same as) cowherd my-server%
    cowherd <env> %proxy%
    cowherd <env> "projectA-app-*" (same as) cowherd projectA-app-%

    So, if you have a configuration file at /etc/cowherd/production.yml, to run with your production
    configuration details you would type:
    
    cowherd production my-container-1


Configuration:
    cowherd reads environment configuration from <env>.json or <env>.yml in ./, /etc/cowherd/ and ~/.cowherd/ folders.

    Multiple config files are suppoerted to enable multi-environment switching.

    If you want to use JSON format, create a config.json in the folders with content:
        {
            "endpoint": "https://rancher.server/v1", // Or "https://rancher.server/v1/projects/xxxx"
            "user": "your_access_key",
            "password": "your_access_password"
        }

    If you want to use YAML format, create a config.yml with content:
        endpoint: https://your.rancher.server/v1 // Or https://rancher.server/v1/projects/xxxx
        user: your_access_key
        password: your_access_password
`
)

// Config holds our endpoint URL and user info.
type Config struct {
	Endpoint string
	User     string
	Password string
}

// RancherAPI si the struct for Rancher connection and auth info.
type RancherAPI struct {
	Endpoint string
	User     string
	Password string
}

// WebTerm is the struct to hold all socket/terminal info.
type WebTerm struct {
	SocketConn *websocket.Conn
	ttyState   *terminal.State
	errChn     chan error
}

func (w *WebTerm) wsWrite() {
	var payload string
	var err error
	var keybuf [1]byte
	for {
		_, err = os.Stdin.Read(keybuf[0:1])
		if err != nil {
			w.errChn <- err
			return
		}

		payload = base64.StdEncoding.EncodeToString(keybuf[0:1])
		err = w.SocketConn.WriteMessage(websocket.BinaryMessage, []byte(payload))
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				w.errChn <- nil
			} else {
				w.errChn <- err
			}
			return
		}
	}
}

func (w *WebTerm) wsRead() {
	var err error
	var raw []byte
	var out []byte
	for {
		_, raw, err = w.SocketConn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				w.errChn <- nil
			} else {
				w.errChn <- err
			}
			return
		}
		out, err = base64.StdEncoding.DecodeString(string(raw))
		if err != nil {
			w.errChn <- err
			return
		}
		os.Stdout.Write(out)
	}
}

// SetRawtty is pretty self explanatory.
func (w *WebTerm) SetRawtty(isRaw bool) {
	if isRaw {
		state, err := terminal.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			panic(err)
		}
		w.ttyState = state
	} else {
		terminal.Restore(int(os.Stdin.Fd()), w.ttyState)
	}
}

// Run is our main websocket thread.
func (w *WebTerm) Run() {
	w.errChn = make(chan error)
	w.SetRawtty(true)

	go w.wsRead()
	go w.wsWrite()

	err := <-w.errChn
	w.SetRawtty(false)

	if err != nil {
		panic(err)
	}
}

func (r *RancherAPI) formatEndpoint() string {
	if r.Endpoint[len(r.Endpoint)-1:len(r.Endpoint)] == "/" {
		return r.Endpoint[0 : len(r.Endpoint)-1]
	}
	return r.Endpoint
}

func (r *RancherAPI) makeReq(req *http.Request) (map[string]interface{}, error) {
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth(r.User, r.Password)

	cli := http.Client{}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	var tokenResp map[string]interface{}
	if err = json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}
	return tokenResp, nil
}

func (r *RancherAPI) containerURL(name string) string {
	req, _ := http.NewRequest("GET", r.formatEndpoint()+"/containers/", nil)
	q := req.URL.Query()
	q.Add("name_like", strings.Replace(name, "*", "%", -1))
	q.Add("state", "running")
	q.Add("kind", "container")
	req.URL.RawQuery = q.Encode()
	resp, err := r.makeReq(req)
	if err != nil {
		fmt.Println("Failure communicating with rancher API: " + err.Error())
		os.Exit(1)
	}
	data := resp["data"].([]interface{})
	var choice = 1
	if len(data) == 0 {
		fmt.Println("Container " + name + " not found, not running, or you don't have access permissions.")
		os.Exit(1)
	}
	if len(data) > 1 {
		fmt.Println("We found more than one containers in system:")
		for i, _ctn := range data {
			ctn := _ctn.(map[string]interface{})
			if _, ok := ctn["data"]; ok {
				fmt.Println(fmt.Sprintf(
					"[%d] %s, Container ID %s in project %s, IP Address %s on Host %s",
					i+1,
					ctn["name"].(string),
					ctn["id"].(string),
					ctn["accountId"].(string),
					ctn["data"].(map[string]interface{})["fields"].(map[string]interface{})["primaryIpAddress"].(string),
					ctn["data"].(map[string]interface{})["fields"].(map[string]interface{})["dockerHostIp"].(string),
				))
			} else {
				fmt.Println(fmt.Sprintf(
					"[%d] %s, Container ID %s in project %s, IP Address %s",
					i+1,
					ctn["name"].(string),
					ctn["id"].(string),
					ctn["accountId"].(string),
					ctn["primaryIpAddress"].(string),
				))
			}
		}
		fmt.Println("--------------------------------------------")
		fmt.Print("Which one you wanna use?: ")
		fmt.Scan(&choice)
	}
	ctn := data[choice-1].(map[string]interface{})
	if _, ok := ctn["data"]; ok {
		fmt.Println(fmt.Sprintf(
			"Target Container: %s, ID %s in project %s, Addr %s on Host %s",
			ctn["name"].(string),
			ctn["id"].(string),
			ctn["accountId"].(string),
			ctn["data"].(map[string]interface{})["fields"].(map[string]interface{})["primaryIpAddress"].(string),
			ctn["data"].(map[string]interface{})["fields"].(map[string]interface{})["dockerHostIp"].(string),
		))
	} else {
		fmt.Println(fmt.Sprintf(
			"Target Container: %s, ID %s in project %s, Addr %s",
			ctn["name"].(string),
			ctn["id"].(string),
			ctn["accountId"].(string),
			ctn["primaryIpAddress"].(string),
		))
	}
	return r.formatEndpoint() + fmt.Sprintf(
		"/containers/%s/", ctn["id"].(string))
}

func (r *RancherAPI) getContainerWsURL(url string) string {
	cols, rows, _ := terminal.GetSize(int(os.Stdin.Fd()))
	req, _ := http.NewRequest("POST", url+"?action=execute",
		strings.NewReader(fmt.Sprintf(
			`{"attachStdin":true, "attachStdout":true,`+
				`"command":["/bin/sh", "-c", "TERM=xterm-256color; export TERM; `+
				`stty cols %d rows %d; `+
				`[ -x /bin/bash ] && ([ -x /usr/bin/script ] && /usr/bin/script -q -c \"/bin/bash\" /dev/null || exec /bin/bash) || exec /bin/sh"], "tty":true}`, cols, rows)))
	resp, err := r.makeReq(req)
	if err != nil {
		fmt.Println("Failed to get access token: ", err.Error())
		os.Exit(1)
	}
	return resp["url"].(string) + "?token=" + resp["token"].(string)
}

func (r *RancherAPI) getWSConn(wsURL string) *websocket.Conn {
	endpoint := r.formatEndpoint()
	header := http.Header{}
	header.Add("Origin", endpoint)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		fmt.Println("We couldn't connect to this node: ", err.Error())
		os.Exit(1)
	}

	pingInterval := 5 * time.Second

	go func() {
		c := time.Tick(pingInterval)

		for _ = range c {
			//fmt.Println("Sending ping")
			if err := conn.WriteControl(
				websocket.PingMessage,
				[]byte("ping"),
				time.Now().Add(1*time.Second),
			); err != nil {
				log.Print("Error in sending ping-pong with server socket")
			}
		}

	}()

	conn.SetPingHandler(func(message string) error {
		err := conn.WriteControl(websocket.PongMessage, []byte(message), time.Now().Add(1*time.Second))

		if err == websocket.ErrCloseSent {
			return nil
		} else if e, ok := err.(net.Error); ok && e.Temporary() {
			return nil
		}
		return err
	})

	return conn
}

// GetContainerConn takes the name parameter, finds the websocket URL, sets up the websocket connection
// to Rancher and then returns it.
func (r *RancherAPI) GetContainerConn(name string) *websocket.Conn {
	fmt.Println("Searching for container " + name)
	url := r.containerURL(name)
	fmt.Println("Getting access token")
	wsurl := r.getContainerWsURL(url)
	fmt.Println("SSH into container ...")
	return r.getWSConn(wsurl)
}

// ReadConfig uses Viper config library to search for Cowherd configs and decode them
// to the Config struct for setting up Rancher connections and Auth.
func ReadConfig(env string) *Config {

	viper.SetConfigName(env)
	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME/")
	viper.AddConfigPath("$HOME/.cowherd")
	viper.AddConfigPath("/etc/cowherd/")
	viper.ReadInConfig()

	endpoint := viper.GetString("endpoint")
	user := viper.GetString("user")
	password := viper.GetString("password")

	fmt.Print(viper.ConfigFileUsed())
	fmt.Print(viper.AllSettings())

	viper.SetEnvPrefix("cowherd")
	viper.AutomaticEnv()

	if endpoint == "" || user == "" || password == "" {
		fmt.Println(USAGE)
		os.Exit(1)
	}

	return &Config{
		Endpoint: endpoint,
		User:     user,
		Password: password,
	}

}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func main() {

	if len(os.Args) != 3 {
		fmt.Printf("NOT ENOUGH ARGUMENTS: HAVE %d, NEED 2", len(os.Args)-1)
		fmt.Println(USAGE)
		os.Exit(1)
	}

	env := os.Args[1]
	name := os.Args[2]

	config := ReadConfig(env)
	rancher := RancherAPI{
		Endpoint: config.Endpoint,
		User:     config.User,
		Password: config.Password,
	}

	var conn *websocket.Conn

	conn = rancher.GetContainerConn(name)

	wt := WebTerm{
		SocketConn: conn,
	}
	wt.Run()

	fmt.Println("Good bye.")
}
