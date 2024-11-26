package main

// derived from https://raw.githubusercontent.com/OrfeasZ/telegraf/b8a4e6cb7aef4134bd506cb873c8040c084a2406/plugins/inputs/powerdns_recursor/protocol_commons.go
// license: MIT

import (
	"fmt"
	"net"
	"strconv"
)

// This below is generally unsafe but necessary in this case
// since the powerdns protocol encoding is host dependent.
// The C implementation uses size_t as the size type for the
// command length. The size and endianness of size_t change
// depending on the platform the program is being run on.
// Using the target architecture endianness and the known
// integer size, we can "recreate" the corresponding C
// behavior in an effort to maintain compatibility. Of course
// in cases where one program is compiled for i386 and the
// other for amd64 (and similar), this method will fail.

const uintSizeInBytes = strconv.IntSize / 8

func writeNativeUIntToConn(conn net.Conn, value uint) error {
	intData := make([]byte, uintSizeInBytes)

	switch uintSizeInBytes {
	case 4:
		HostEndianness.PutUint32(intData, uint32(value))
	case 8:
		HostEndianness.PutUint64(intData, uint64(value))
	default:
		return fmt.Errorf("unsupported system configuration")
	}

	_, err := conn.Write(intData)
	return err
}

func readNativeUIntFromConn(conn net.Conn) (uint, error) {
	intData := make([]byte, uintSizeInBytes)

	n, err := conn.Read(intData)

	if err != nil {
		return 0, err
	}

	if n != uintSizeInBytes {
		return 0, fmt.Errorf("did not read enough data for native uint: read '%v' bytes, expected '%v'", n, uintSizeInBytes)
	}

	switch uintSizeInBytes {
	case 4:
		return uint(HostEndianness.Uint32(intData)), nil
	case 8:
		return uint(HostEndianness.Uint64(intData)), nil
	default:
		return 0, fmt.Errorf("unsupported system configuration")
	}
}
