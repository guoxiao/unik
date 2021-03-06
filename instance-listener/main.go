package main

import (
	"encoding/json"
	"github.com/emc-advanced-dev/pkg/errors"
	"flag"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/go-martini/martini"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const statefile = "statefile.json"

type state struct {
	MacIpMap  map[string]string            `json:"Ips"`
	MacEnvMap map[string]map[string]string `json:"Envs"`
}

func main() {
	verbose := flag.Bool("v", false, "verbose mode")
	flag.Parse()
	if *verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}
	ipMapLock := sync.RWMutex{}
	envMapLock := sync.RWMutex{}
	saveLock := sync.Mutex{}
	var s state
	s.MacIpMap = make(map[string]string)
	s.MacEnvMap = make(map[string]map[string]string)

	data, err := ioutil.ReadFile(statefile)
	if err != nil {
		logrus.WithError(err).Warnf("could not read statefile, maybe this is first boot")
	} else {
		if err := json.Unmarshal(data, &s); err != nil {
			logrus.WithError(err).Warnf("failed to parse state json")
		}
	}

	listenerIp, err := getLocalIp()
	if err != nil {
		logrus.Fatalf("failed to get local ip: %v", err)
	}

	logrus.Infof("Starting unik discovery (udp heartbeat broadcast) with ip %s", listenerIp.String())
	info := []byte("unik:" + listenerIp.String())
	listenerIpMask := listenerIp.DefaultMask()
	BROADCAST_IPv4 := reverseMask(listenerIp, listenerIpMask)
	if listenerIpMask == nil {
		logrus.WithFields(logrus.Fields{"listener-ip": listenerIp, "listener-ip-mask": listenerIpMask}).Fatalf("could not calculate broadcast address")
	}
	socket, err := net.DialUDP("udp4", nil, &net.UDPAddr{
		IP:   BROADCAST_IPv4,
		Port: 9876,
	})
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"broadcast-ip": BROADCAST_IPv4,
		}).Fatalf("failed to dial udp broadcast connection")
	}
	go func() {
		for {
			_, err = socket.Write(info)
			if err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"broadcast-ip": BROADCAST_IPv4,
				}).Fatalf("failed writing to broadcast udp socket")
			}
			logrus.Debugf("broadcasting...")
			time.Sleep(2000 * time.Millisecond)
		}
	}()
	m := martini.Classic()
	m.Post("/register", func(res http.ResponseWriter, req *http.Request) {
		splitAddr := strings.Split(req.RemoteAddr, ":")
		if len(splitAddr) < 1 {
			logrus.WithFields(logrus.Fields{
				"req.RemoteAddr": req.RemoteAddr,
			}).Errorf("could not parse remote addr into ip/port combination")
			return
		}
		instanceIp := splitAddr[0]
		macAddress := req.URL.Query().Get("mac_address")
		logrus.WithFields(logrus.Fields{
			"Ip":          instanceIp,
			"mac-address": macAddress,
		}).Infof("Instance registered")
		//mac address = the instance id in vsphere/vbox
		go func() {
			ipMapLock.Lock()
			defer ipMapLock.Unlock()
			s.MacIpMap[macAddress] = instanceIp
			go save(s, saveLock)
		}()
		envMapLock.RLock()
		defer envMapLock.RUnlock()
		env, ok := s.MacEnvMap[macAddress]
		if !ok {
			env = make(map[string]string)
			logrus.WithFields(logrus.Fields{"mac": macAddress, "envmap": s.MacEnvMap}).Errorf("no env set for instance, replying with empty map")
		}
		data, err := json.Marshal(env)
		if err != nil {
			logrus.WithError(err).Errorf("could not marshal env to json")
			return
		}
		logrus.Debugf("responding with data: %s", data)
		fmt.Fprintf(res, "%s", data)
	})
	m.Post("/set_instance_env", func(res http.ResponseWriter, req *http.Request) (string, error) {
		macAddress := req.URL.Query().Get("mac_address")
		data, err := ioutil.ReadAll(req.Body)
		if err != nil {
			return "failed to read req body\n", err
		}
		defer req.Body.Close()
		var env map[string]string
		if err := json.Unmarshal(data, &env); err != nil {
			return "failed to unmarshal data " + string(data) + " to map[string]string\n", err
		}
		logrus.WithFields(logrus.Fields{
			"env":         env,
			"mac-address": macAddress,
		}).Infof("Env set for instance")
		envMapLock.Lock()
		defer envMapLock.Unlock()
		s.MacEnvMap[macAddress] = env
		go save(s, saveLock)
		return "success\n", nil
	})
	m.Get("/instances", func() (string, error) {
		ipMapLock.RLock()
		defer ipMapLock.RUnlock()
		data, err := json.Marshal(s.MacIpMap)
		if err != nil {
			return "", err
		}
		return string(data), nil
	})
	m.RunOnAddr(":3000")
}

func getLocalIp() (net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return net.IP{}, errors.New("retrieving network interfaces" + err.Error())
	}
	for _, iface := range ifaces {
		logrus.Infof("found an interface: %v\n", iface)
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			logrus.WithField("addr", addr).Debugf("inspecting address")
			switch v := addr.(type) {
			case *net.IPNet:
				if !v.IP.IsLoopback() && v.IP.IsGlobalUnicast() && v.IP.To4() != nil {
					return v.IP.To4(), nil
				}
			}
		}
	}
	return net.IP{}, errors.New("failed to find ip on ifaces: " + fmt.Sprintf("%v", ifaces))
}

// ReverseMask returns the result of masking the IP address ip with mask.
func reverseMask(ip net.IP, mask net.IPMask) net.IP {
	n := len(ip)
	if n != len(mask) {
		return nil
	}
	out := make(net.IP, n)
	for i := 0; i < n; i++ {
		out[i] = ip[i] | (^mask[i])
	}
	return out
}

func save(s state, l sync.Mutex) {
	if err := func() error {
		l.Lock()
		defer l.Unlock()
		data, err := json.Marshal(s)
		if err != nil {
			return err
		}
		if err := ioutil.WriteFile(statefile, data, 0644); err != nil {
			return err
		}
		return nil
	}(); err != nil {
		logrus.WithError(err).Errorf("failed to save state file %s", statefile)
	}
}
