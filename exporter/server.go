// Copyright 2016-2020 The grok_exporter Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package exporter

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"

	config "github.com/fstab/grok_exporter/config/v3"
)

type HttpServerPathHandler struct {
	Path    string
	Handler http.Handler
}

// cert and key created with openssl req -x509 -newkey rsa:2048 -keyout key.pem -out cert.pem -nodes

const defaultCert = `-----BEGIN CERTIFICATE-----
MIIDtTCCAp2gAwIBAgIJAP9eE4ZtnJZnMA0GCSqGSIb3DQEBBQUAMEUxCzAJBgNV
BAYTAkFVMRMwEQYDVQQIEwpTb21lLVN0YXRlMSEwHwYDVQQKExhJbnRlcm5ldCBX
aWRnaXRzIFB0eSBMdGQwHhcNMTYwMTMxMTUzNzAxWhcNMTYwMzAxMTUzNzAxWjBF
MQswCQYDVQQGEwJBVTETMBEGA1UECBMKU29tZS1TdGF0ZTEhMB8GA1UEChMYSW50
ZXJuZXQgV2lkZ2l0cyBQdHkgTHRkMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIB
CgKCAQEAuGMNuDwIdCbSNnOd0Bl3LejAgYjcXb6I2cdZCISt45CERO/mvn59MLYj
P7awhZuOBYhu00WshNfonbyZ7mNgTIIe8MuRIbHvqVhb2i8CwleorJhzT6cnfFf5
xjXj8DclSZDJAqQzthvGra7F67G38bKjl/0tx4T7Z4shp4d+M9to4zQp5x3xZ6hj
/3J9oZiMbAy8s+kODqIPHCsVjiCQqr/649tF5Fiq+UGRcOzR2471xKqhB37nMfAz
uoE5P6HENGN4K+fG8yJ7biBz063GZbcjopIj7RSZK9eZGfzGZm0NoqOUyjsyCKpS
0teK4Frw6Um8wTPkRdOypNvVgchDTQIDAQABo4GnMIGkMB0GA1UdDgQWBBRm4vRi
X6QudIwhw76HQRQQLj7knTB1BgNVHSMEbjBsgBRm4vRiX6QudIwhw76HQRQQLj7k
naFJpEcwRTELMAkGA1UEBhMCQVUxEzARBgNVBAgTClNvbWUtU3RhdGUxITAfBgNV
BAoTGEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZIIJAP9eE4ZtnJZnMAwGA1UdEwQF
MAMBAf8wDQYJKoZIhvcNAQEFBQADggEBAK8lJOLaSgr0rGt3tLQ5lDG28L1tK2ki
TZNGXvbZOupdnQPC90gP0EiiNXDLIqqYYe12tA3YV6fQ+PhAaUdC413njHQGtbmn
2P/uLfHycrsttUpqmbWDF+uCdW/z42jztxMg4Ett2mMgt0LyQkyBxOCCi3Ia8cy9
GttqN9uvNsyfCVzBMC8DLpmnhBMtGJ2to0C/ktyzM8Z2t8I6F/RJUKiC/qAaqvHa
mrYEF+Y3rJCXlZoRCyd8j2lT2VSRFoOv+LgOV4aUgLd8Hw+epRJ/I+gO5YoQ1n6x
tEOmZmjlXW1eoFDTp2jnVN4gL7u4/d4B7F0kCltb3/3ZtWTv8AeTDSE=
-----END CERTIFICATE-----
`

const defaultKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEpQIBAAKCAQEAuGMNuDwIdCbSNnOd0Bl3LejAgYjcXb6I2cdZCISt45CERO/m
vn59MLYjP7awhZuOBYhu00WshNfonbyZ7mNgTIIe8MuRIbHvqVhb2i8CwleorJhz
T6cnfFf5xjXj8DclSZDJAqQzthvGra7F67G38bKjl/0tx4T7Z4shp4d+M9to4zQp
5x3xZ6hj/3J9oZiMbAy8s+kODqIPHCsVjiCQqr/649tF5Fiq+UGRcOzR2471xKqh
B37nMfAzuoE5P6HENGN4K+fG8yJ7biBz063GZbcjopIj7RSZK9eZGfzGZm0NoqOU
yjsyCKpS0teK4Frw6Um8wTPkRdOypNvVgchDTQIDAQABAoIBAQCGsf18v4Yha5aW
poD7Ww7/3455UgRBCwYnqQO2QE5S9ehZ/7JdKEPFyNgZHBj5kTf/fLoQ5k3vwVWx
nOwKBFh9q3R0zRCZP8XmvKBk04C9fZG/e6KI5n/mytGw5P89JNu9UOI2ZsNL3iCW
Eh2NXwcTrj7psc62eMO60R1lp4oe0ITyKEhIbHWK1cu+rRpmpIOOjdZK8fz296bX
HdM/tIw6OhXun6dEMRAY1pTy5Eznpvi0mIp+o8pGovm0KUokf87MTDi4GMnm14/7
znej5i6ETWYk1Yy1n/nzf+6OiAg66qu1TX76d/8w1JmY//LWpXuk3xEr/ZDBNMtk
/PrYswpBAoGBANm+iwfVssuyC5hcTva0InycU6jaKjPIt+Q7OWBAKR/EXj0CMpni
WYYR7M9Mcous2J1Rf8tTJr9K5RMheHY3zGn6qHYirUj8wDF/hlhvNG6RFME1U9NT
dfWBiD4JKDoTEzABDJQm6IroZpfN/89O1SuxzPuaLx4LVlY4eRP8ocTRAoGBANjI
MzPI+wlv2FFq9LjbNiD95lfIhp2xumrhmoplM5T9YlAiymtGU9fbA0GwKEqDlaqf
niXenkZhLclUbc6u85hOfmmoq5pEd6xk/OUstrd5dAxQMCRGlXMcEBYvazVbqUTp
5PbJ3FX7eKDxRi/AIKWV+DY2289yyF6BL0HjW+W9AoGAXlP8WNWL0lB8U3HRx3A7
7G2wlFqGo85VU6sQbRD+f8OK67UTBLUZAUqsoxVEHhwv7t8KlKOeCorAeCwsylHb
3SF4b00QcqkD/a14HsF2Hlv9eMHIYakrVcLaqb0/zwDKdCZQM7IzVVHed+8G3eER
2g75dRnTRZm1uj5WvYDY97ECgYEAuMC20pWhTYOiypDrDHjXAvsgywO9prwH8ntf
qD9j3MCufzmHZjHD1x1zAxLM4+SNM6Nht0ipf7XmvcVU6Gc2eEG9fvMffRSJIcXX
usGG34uFGdFlliUJzdbG5wF2zzzVYEQuvR2AyU7OmevHM3780+KibiIG6CAdIF3d
FrxcX8kCgYEAh+9YpiAVaYGDiAiqUMFRlQuDod/XN8HVreAHHn5G+A5O8yQMjB4T
/o2pN0JuECBcuVTcxl94w6kmfX1fQS3q7j+gZMzlO+eVweuLC19gO96RPUETxgeV
XLgD9hrDBrTbnKBHHQ6MHpT6ILi4w/e4+5XEUUOBf44ZJE71uRr4ZUA=
-----END RSA PRIVATE KEY-----
`

func RunHttpsServer(cfg config.ServerConfig, httpHandlers []HttpServerPathHandler) error {
	err := tryOpenPort(cfg.Host, cfg.Port)
	if err != nil {
		return listenFailedError(cfg.Host, cfg.Port, err)
	}
	for _, httpHandler := range httpHandlers {
		http.Handle(httpHandler.Path, httpHandler.Handler)
	}
	tlsCfg, err := makeTLSConfig(cfg)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:      fmt.Sprintf("%v:%v", cfg.Host, cfg.Port),
		TLSConfig: tlsCfg,
	}
	return server.ListenAndServeTLS("", "")
}

func RunHttpServer(host string, port int, httpHandlers []HttpServerPathHandler) error {
	err := tryOpenPort(host, port)
	if err != nil {
		return listenFailedError(host, port, err)
	}
	for _, httpHandler := range httpHandlers {
		http.Handle(httpHandler.Path, httpHandler.Handler)
	}

	return http.ListenAndServe(fmt.Sprintf("%v:%v", host, port), nil)
}

// Golang's http.ListenAndServe() has an unexpected behaviour when the port is in use:
// Instead of returning an error, it tries to open an IPv6-only listener.
// If this works (because the other application on that port is IPv4-only), no error is returned.
// This is confusing for the user, we want an error if the IPv4 port is in use.
func tryOpenPort(host string, port int) error {
	ln, err := net.Listen("tcp4", fmt.Sprintf("%v:%v", host, port))
	if err != nil {
		return err
	}
	return ln.Close()
}

func listenFailedError(host string, port int, err error) error {
	if len(host) > 0 {
		return fmt.Errorf("cannot bind to %v:%v: %v", host, port, err)
	} else {
		return fmt.Errorf("cannot open port %v: %v", port, err)
	}
}

func createTempFile(prefix string, data []byte) (string, error) {
	tempFile, err := ioutil.TempFile(os.TempDir(), prefix)
	if err != nil {
		return "", fmt.Errorf("Failed to create temporary file: %v", err.Error())
	}
	_, err = tempFile.Write(data)
	if err != nil {
		return "", fmt.Errorf("Failed to write temporary file: %v", err.Error())
	}
	err = tempFile.Close()
	if err != nil {
		return "", fmt.Errorf("Failed to close temporary file: %v", err.Error())
	}
	return tempFile.Name(), nil
}

// Assuming serverCfg is valid.
func makeTLSConfig(cfg config.ServerConfig) (*tls.Config, error) {
	var (
		result = &tls.Config{}
		cert   tls.Certificate
		bytes  []byte
		err    error
	)
	if len(cfg.Cert) == 0 && len(cfg.Key) == 0 {
		cert, err = tls.X509KeyPair([]byte(defaultCert), []byte(defaultKey))
		if err != nil {
			return nil, fmt.Errorf("unexpected error initializing the built-in SSL certificates: %v", err)
		}
	} else if len(cfg.Cert) > 0 && len(cfg.Key) > 0 {
		cert, err = tls.LoadX509KeyPair(cfg.Cert, cfg.Key)
		if err != nil {
			return nil, fmt.Errorf("error reading SSL cert or key file: %v", err)
		}
	}
	result.Certificates = append(result.Certificates, cert)

	if len(cfg.ClientCA) > 0 {
		result.ClientCAs = x509.NewCertPool()
		bytes, err = ioutil.ReadFile(cfg.ClientCA)
		if err != nil {
			return nil, fmt.Errorf("failed to read client_ca file: %v", err)
		}
		if !result.ClientCAs.AppendCertsFromPEM(bytes) {
			return nil, fmt.Errorf("failed to read certificates from the client_ca file")
		}
	}

	if len(cfg.Ciphers) > 0 {
		var idc uint16
		var cf []uint16
		var cfstring []string
		for _, cfg := range cfg.Ciphers {
			for _, cs := range tls.CipherSuites() {
				if cs.Name == cfg {
					idc = (cs.ID)
					cfstring = append(cfstring, cfg)
				}
				cf = append(cf, idc)
			}
		}
		fmt.Println("ciphers loaded: ", cfstring)
		if len(cf) > 0 {
			result.CipherSuites = cf
		}
	}

	if len(cfg.MinVersion) > 0 {
		var tlsVersions = map[string]uint16{
			"TLS13": (uint16)(tls.VersionTLS13),
			"TLS12": (uint16)(tls.VersionTLS12),
			"TLS11": (uint16)(tls.VersionTLS11),
			"TLS10": (uint16)(tls.VersionTLS10),
		}
		if v, ok := tlsVersions[cfg.MinVersion]; ok {
			result.MinVersion = v
			fmt.Println("min tls version : ", cfg.MinVersion)
		}
	}

	switch cfg.ClientAuth {
	case "RequestClientCert":
		result.ClientAuth = tls.RequestClientCert
	case "RequireClientCert":
		result.ClientAuth = tls.RequireAnyClientCert
	case "VerifyClientCertIfGiven":
		result.ClientAuth = tls.VerifyClientCertIfGiven
	case "RequireAndVerifyClientCert":
		result.ClientAuth = tls.RequireAndVerifyClientCert
	case "", "NoClientCert":
		result.ClientAuth = tls.NoClientCert
	}
	return result, nil
}
