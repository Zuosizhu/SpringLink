package codec

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

var xorKey = []byte("SpringLink2025!")

type ConnectionCode struct {
	Version       int    `json:"v"`
	Name          string `json:"n"`
	Protocol      string `json:"p"`
	ConnectMethod string `json:"c"`
	Transport     string `json:"t"`
	LocalPort     int    `json:"lp"`
	RemoteHost    string `json:"rh"`
	RemotePort    int    `json:"rp"`
	WstunnelPort  int    `json:"wp,omitempty"`
	PublicIP      string `json:"ip,omitempty"`
	ServAddr      string `json:"sa,omitempty"`
	FrpsPort      int    `json:"fp,omitempty"`
	FrpsToken     string `json:"ft,omitempty"`
}

const (
	flagWstunnelPort = 1 << iota
	flagPublicIP
	flagServAddr
	flagFrpsPort
	flagFrpsToken
)

func xor(data []byte) []byte {
	out := make([]byte, len(data))
	for i, b := range data {
		out[i] = b ^ xorKey[i%len(xorKey)]
	}
	return out
}

func Encode(code *ConnectionCode) (string, error) {
	data := marshalBinary(code)
	obfuscated := xor(data)

	dst := make([]byte, ascii85.MaxEncodedLen(len(obfuscated)))
	n := ascii85.Encode(dst, obfuscated)

	return "slink://" + string(dst[:n]), nil
}

func Decode(s string) (*ConnectionCode, error) {
	if !strings.HasPrefix(s, "slink://") {
		return nil, fmt.Errorf("invalid connection code prefix")
	}
	raw := s[len("slink://"):]

	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		dst := make([]byte, len(raw))
		nd, _, err := ascii85.Decode(dst, []byte(raw), true)
		if err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}
		decoded = dst[:nd]
	}

	data := xor(decoded)

	if code, err := decodeZlib(data); err == nil {
		return code, nil
	}

	if len(data) > 0 && data[0] == '{' {
		var code ConnectionCode
		if err := json.Unmarshal(data, &code); err != nil {
			return nil, fmt.Errorf("json unmarshal: %w", err)
		}
		return &code, nil
	}

	code, err := unmarshalBinary(data)
	if err != nil {
		return nil, fmt.Errorf("binary decode: %w", err)
	}
	return code, nil
}

func decodeZlib(data []byte) (*ConnectionCode, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var code ConnectionCode
	if err := json.Unmarshal(raw, &code); err != nil {
		return nil, err
	}
	return &code, nil
}

func marshalBinary(c *ConnectionCode) []byte {
	buf := new(bytes.Buffer)

	buf.WriteByte(byte(c.Version))

	var flags byte
	if c.WstunnelPort != 0 {
		flags |= flagWstunnelPort
	}
	if c.PublicIP != "" {
		flags |= flagPublicIP
	}
	if c.ServAddr != "" {
		flags |= flagServAddr
	}
	if c.FrpsPort != 0 {
		flags |= flagFrpsPort
	}
	if c.FrpsToken != "" {
		flags |= flagFrpsToken
	}
	buf.WriteByte(flags)

	writeString(buf, c.Name)
	writeString(buf, c.Protocol)
	writeString(buf, c.ConnectMethod)
	writeString(buf, c.Transport)
	writeString(buf, c.RemoteHost)

	writeUint16(buf, c.LocalPort)
	writeUint16(buf, c.RemotePort)

	if flags&flagWstunnelPort != 0 {
		writeUint16(buf, c.WstunnelPort)
	}
	if flags&flagPublicIP != 0 {
		writeString(buf, c.PublicIP)
	}
	if flags&flagServAddr != 0 {
		writeString(buf, c.ServAddr)
	}
	if flags&flagFrpsPort != 0 {
		writeUint16(buf, c.FrpsPort)
	}
	if flags&flagFrpsToken != 0 {
		writeString(buf, c.FrpsToken)
	}

	return buf.Bytes()
}

func unmarshalBinary(data []byte) (*ConnectionCode, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("data too short")
	}

	c := &ConnectionCode{
		Version: int(data[0]),
	}
	flags := data[1]
	off := 2

	var err error
	c.Name, off, err = readString(data, off)
	if err != nil {
		return nil, err
	}
	c.Protocol, off, err = readString(data, off)
	if err != nil {
		return nil, err
	}
	c.ConnectMethod, off, err = readString(data, off)
	if err != nil {
		return nil, err
	}
	c.Transport, off, err = readString(data, off)
	if err != nil {
		return nil, err
	}
	c.RemoteHost, off, err = readString(data, off)
	if err != nil {
		return nil, err
	}

	c.LocalPort, off, err = readUint16(data, off)
	if err != nil {
		return nil, err
	}
	c.RemotePort, off, err = readUint16(data, off)
	if err != nil {
		return nil, err
	}

	if flags&flagWstunnelPort != 0 {
		c.WstunnelPort, off, err = readUint16(data, off)
		if err != nil {
			return nil, err
		}
	}
	if flags&flagPublicIP != 0 {
		c.PublicIP, off, err = readString(data, off)
		if err != nil {
			return nil, err
		}
	}
	if flags&flagServAddr != 0 {
		c.ServAddr, off, err = readString(data, off)
		if err != nil {
			return nil, err
		}
	}
	if flags&flagFrpsPort != 0 {
		c.FrpsPort, off, err = readUint16(data, off)
		if err != nil {
			return nil, err
		}
	}
	if flags&flagFrpsToken != 0 {
		c.FrpsToken, off, err = readString(data, off)
		if err != nil {
			return nil, err
		}
	}

	return c, nil
}

func writeString(buf *bytes.Buffer, s string) {
	l := len(s)
	buf.WriteByte(byte(l >> 8))
	buf.WriteByte(byte(l))
	buf.WriteString(s)
}

func writeUint16(buf *bytes.Buffer, v int) {
	buf.WriteByte(byte(v >> 8))
	buf.WriteByte(byte(v))
}

func readUint16(data []byte, off int) (int, int, error) {
	if off+2 > len(data) {
		return 0, 0, fmt.Errorf("unexpected end of data")
	}
	v := int(data[off])<<8 | int(data[off+1])
	return v, off + 2, nil
}

func readString(data []byte, off int) (string, int, error) {
	l, off, err := readUint16(data, off)
	if err != nil {
		return "", 0, err
	}
	if off+l > len(data) {
		return "", 0, fmt.Errorf("unexpected end of data")
	}
	s := string(data[off : off+l])
	return s, off + l, nil
}
