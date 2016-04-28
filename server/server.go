package server

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

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

func RunWithDefaultKeys(port int, path string, handler http.Handler) error {
	cert, err := createTempFile("cert", []byte(defaultCert))
	if err != nil {
		return err
	}
	defer os.Remove(cert)
	key, err := createTempFile("key", []byte(defaultKey))
	if err != nil {
		return err
	}
	defer os.Remove(key)
	return Run(port, cert, key, path, handler)
}

func Run(port int, cert, key, path string, handler http.Handler) error {
	http.Handle(path, handler)
	return http.ListenAndServeTLS(fmt.Sprintf(":%v", port), cert, key, nil)
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
