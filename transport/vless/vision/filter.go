package vision

import (
	"bytes"
	"encoding/binary"

	"github.com/metacubex/mihomo/log"
)

var (
	tls13SupportedVersions  = []byte{0x00, 0x2b, 0x00, 0x02, 0x03, 0x04}
	tlsClientHandshakeStart = []byte{0x16, 0x03}
	tlsServerHandshakeStart = []byte{0x16, 0x03, 0x03}
	tlsApplicationDataStart = []byte{0x17, 0x03, 0x03}

	tls13CipherSuiteMap = map[uint16]string{
		0x1301: "TLS_AES_128_GCM_SHA256",
		0x1302: "TLS_AES_256_GCM_SHA384",
		0x1303: "TLS_CHACHA20_POLY1305_SHA256",
		0x1304: "TLS_AES_128_CCM_SHA256",
		0x1305: "TLS_AES_128_CCM_8_SHA256",
	}
)

const (
	tlsHandshakeTypeClientHello byte = 0x01
	tlsHandshakeTypeServerHello byte = 0x02
)

func (vc *Conn) FilterTLS(buffer []byte) (index int) {
	if vc.packetsToFilter.Load() <= 0 {
		return 0
	}
	lenP := len(buffer)
	vc.packetsToFilter.Add(-1)
	if index = bytes.Index(buffer, tlsServerHandshakeStart); index != -1 {
		if lenP > index+5 {
			if buffer[0] == 22 && buffer[1] == 3 && buffer[2] == 3 {
				vc.isTLS.Store(true)
				if buffer[5] == tlsHandshakeTypeServerHello {
					//log.Debugln("isTLS12orAbove")
					vc.remainingServerHello.Store(uint32(binary.BigEndian.Uint16(buffer[index+3:])) + 5)
					vc.isTLS12orAbove.Store(true)
					if lenP-index >= 79 && vc.remainingServerHello.Load() >= 79 {
						sessionIDLen := int(buffer[index+43])
						cipherOffset := index + 43 + sessionIDLen + 1
						if cipherOffset+2 <= lenP {
							vc.cipher.Store(uint32(binary.BigEndian.Uint16(buffer[cipherOffset:])))
						}
					}
				}
			}
		}
	} else if index = bytes.Index(buffer, tlsClientHandshakeStart); index != -1 {
		if lenP > index+5 && buffer[index+5] == tlsHandshakeTypeClientHello {
			vc.isTLS.Store(true)
		}
	}

	if remaining := vc.remainingServerHello.Load(); remaining > 0 {
		end := int(remaining)
		i := index
		if i < 0 {
			i = 0
		}
		if i+end > lenP {
			end = lenP
			vc.remainingServerHello.Add(-uint32(end - i))
		} else {
			vc.remainingServerHello.Add(-uint32(end))
			end += i
		}
		if bytes.Contains(buffer[i:end], tls13SupportedVersions) {
			// TLS 1.3 Client Hello
			cs, ok := tls13CipherSuiteMap[uint16(vc.cipher.Load())]
			if ok && cs != "TLS_AES_128_CCM_8_SHA256" {
				vc.enableXTLS.Store(true)
			}
			log.Debugln("XTLS Vision found TLS 1.3, packetLength=%d， CipherSuite=%s", lenP, cs)
			vc.packetsToFilter.Store(0)
			return
		} else if vc.remainingServerHello.Load() <= 0 {
			log.Debugln("XTLS Vision found TLS 1.2, packetLength=%d", lenP)
			vc.packetsToFilter.Store(0)
			return
		}
		log.Debugln("XTLS Vision found inconclusive server hello, packetLength=%d, remainingServerHelloBytes=%d", lenP, vc.remainingServerHello.Load())
	}
	if vc.packetsToFilter.Load() <= 0 {
		log.Debugln("XTLS Vision stop filtering")
	}
	return
}
