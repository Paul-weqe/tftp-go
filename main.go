package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"syscall"
)

type ServerDetails struct {
	ipAddress net.IP
	port      uint16
	fileName  string
	mode      string
}

func getServerDetails(server *ServerDetails) error {

	fmt.Println("Enter details of server to connect to")
	fmt.Printf("IP Address: ")
	var ip string
	fmt.Scan(&ip)

	server.ipAddress = net.ParseIP(ip)
	if server.ipAddress == nil {
		return errors.New("Invalid IP Address")
	}
	fmt.Printf("Port Number: ")
	fmt.Scan(&server.port)

	fmt.Printf("Filename: ")
	fmt.Scan(&server.fileName)

	fmt.Printf("Mode[octet/netascii]: ")
	fmt.Scan(&server.mode)

	if (server.mode != "octet") && (server.mode != "netascii") {
		return errors.New(
			"Invalid modes. Only valid modes: [octet, netascii]",
		)
	}
	return nil
}

func runClient() {

	fd, fdErr := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if fdErr != nil {
		log.Fatal("Socket Creation error:", fdErr)
		return
	}
	defer syscall.Close(fd)

	serverDetails := ServerDetails{}
	getServerErr := getServerDetails(&serverDetails)
	if getServerErr != nil {
		fmt.Println("Error: ", getServerErr)
	}

	octets := serverDetails.ipAddress.To4()
	if octets == nil {
		fmt.Println("Currently only supports IPv4 addresses")
	}

	sa := syscall.SockaddrInet4{
		Port: int(serverDetails.port),
		Addr: [4]byte(octets),
	}

	/*
	 * 2 bytes        | string    | 1 byte  | string | 1 byte
	 * ------------------------------------------------------
	 *  RRQ/ | 01/02  | Filename  | 0       | Mode   | 0    |
	 *  WRQ -------------------------------------------------
	 *  2  bytes 2 bytes n bytes
	 */
	buffer := []byte{}
	buffer = append(buffer, 00)
	buffer = append(buffer, 01)

	// Add filename to packet.
	for i, _ := range serverDetails.fileName {
		buffer = append(buffer, serverDetails.fileName[i])
	}
	buffer = append(buffer, 0)

	// Add Mode to packet details.
	for i, _ := range serverDetails.mode {
		buffer = append(buffer, serverDetails.mode[i])
	}
	buffer = append(buffer, 0)

	if err := syscall.Sendto(fd, buffer, 0, &sa); err != nil {
		fmt.Println("Problem sending opening TFTP packet")
		return
	}

	var blockNumber uint16 = 1
	var dataSize uint16 = 512
	fmt.Println("Fetching file...")

	file, fileErr := os.Create(serverDetails.fileName)
	if fileErr != nil {
		fmt.Println("File creation error: ", fileErr)
		return
	}

	for {
		recvBuf := make([]byte, 516)
		n, recvSocketAddr, recvErr := syscall.Recvfrom(fd, recvBuf, 0)

		// Smaller than the required packet block
		if n < 5 {
			fmt.Println(n, "is smaller than the required packet TFTP size")
			return
		}

		if recvErr != nil {
			fmt.Println("Receive Error:", recvErr)
			return
		}

		if addr, ok := recvSocketAddr.(*syscall.SockaddrInet4); ok {
			sa.Port = addr.Port
		}

		/*
		 * DATA TFTP PACKET
		 *
		 * 2 bytes    | 2 bytes | n bytes |
		 * --------------------------------
		 *  OPCODE 03 | Block # | Data    |
		 *  -------------------------------
		 */
		opcode := binary.BigEndian.Uint16(recvBuf[:2])

		if opcode == 3 {
			recvBlock := binary.BigEndian.Uint16(recvBuf[2:4])
			if recvBlock != blockNumber {
				fmt.Printf("Expected block %d, got %d\n", blockNumber, recvBlock)
				continue
			}

			data := recvBuf[4:n]
			if _, writeErr := file.Write(data); writeErr != nil {
				fmt.Println("File write error:", writeErr)
			}

			/*
			 * ACK TFTP PACKET
			 * 2 bytes 2 bytes
			 * -------------------
			 *| 04 | Block # |
			 *  --------------------
			 */
			ackBuffer := [4]byte{}
			binary.BigEndian.PutUint16(ackBuffer[0:2], 4)
			binary.BigEndian.PutUint16(ackBuffer[2:4], blockNumber)

			if sendErr := syscall.Sendto(fd, ackBuffer[:], 0, &sa); sendErr != nil {
				fmt.Println("ACK send error:", sendErr)
			}

			dataSize = uint16(len(data))
			if dataSize < 512 {
				fmt.Println("\nFile transfer complete")
				return
			}
			blockNumber += 1
		}
	}
}

func runServer() {
	var port int
	fmt.Printf("Enter Port you'd like the TFTP server to run on: ")
	fmt.Scan(&port)

	fd, fdErr := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if fdErr != nil {
		log.Fatal("Socket creation error", fdErr)
	}
	defer syscall.Close(fd)

	sa := syscall.SockaddrInet4{
		Port: port,
		Addr: [4]byte{0, 0, 0, 0},
	}
	syscall.Bind(fd, &sa)

	for {
		recvBuf := make([]byte, 512)
		_, recvClientSa, recvErr := syscall.Recvfrom(fd, recvBuf, 0)
		if recvErr != nil {
			fmt.Println("Error receiving data from client:", recvErr)
			return
		}

		var blockNumber uint16 = 1
		// Choose random port number from 10000 upwards.
		innerPort := rand.Intn(65535-10000) + 10000
		innerFd, _ := syscall.Socket(
			syscall.AF_INET, syscall.SOCK_DGRAM, 0,
		)
		innerSa := syscall.SockaddrInet4{
			Port: innerPort,
			Addr: [4]byte{0, 0, 0, 0},
		}

		// Fetch our client's socket details.
		clientSa := syscall.SockaddrInet4{
			Port: innerPort,
			Addr: [4]byte{0, 0, 0, 0},
		}
		if addr, ok := recvClientSa.(*syscall.SockaddrInet4); ok {
			clientSa.Port = addr.Port
			clientSa.Addr = addr.Addr
		}

		syscall.Bind(innerFd, &innerSa)
		defer syscall.Close(innerFd)

		/*
		 * -------------------------------------------------------------
		 * 2 bytes            | string    | 1 byte | string  | 1 bytes |
		 * -------------------------------------------------------------
		 *  [RRQ/WRQ] [01/02] | Filename  | 0      | Mode    | 0       |
		 *  ------------------------------------------------------------
		 *  */
		opcode := binary.BigEndian.Uint16(recvBuf[:2])
		var fileName string = ""
		// fileBuf will be the buffer store for the file we are reading from.
		fileBuf := make([]byte, 512)
		var mode string = ""

		// Keeps track of how far we have read the recvBuf.
		var currentPosition uint8 = 2

		if opcode == 1 || opcode == 2 {
			// Fetch the filename.
			for recvBuf[currentPosition] != 0x00 {
				fileName += string(recvBuf[currentPosition])
				currentPosition += 1
			}
			fmt.Println("request received for file", fileName)

			// Skip over the 1 byte.
			currentPosition += 1
			for recvBuf[currentPosition] != 0x00 {
				mode += string(recvBuf[currentPosition])
				currentPosition += 1
			}

			// Open file to be sent to the Client.
			fileFd, fileErr := os.Open(fileName)
			if fileErr != nil {
				fmt.Println("unable to open", fileName)
				return
			}
			defer fileFd.Close()

			fileN, err := fileFd.Read(fileBuf)
			if err != nil && err != io.EOF {
				fmt.Println("Error:", err)
				return
			}

			msgBuffer := GenerateDataBuffer(blockNumber, 03, fileBuf, fileN)
			if sendErr := syscall.Sendto(innerFd, msgBuffer, 0, &clientSa); sendErr != nil {
				fmt.Println("Error sending file: ", sendErr)
			}
			// EOF
			if fileN == 0 {
				return
			}
			for {
				syscall.Recvfrom(innerFd, recvBuf, 0)

				/*
				 * Received ACK packet.
				 *
				 * 2 bytes   | 2 bytes |
				 * ---------------------
				 * OPCODE 04 | Block # |
				 * ---------------------
				 */
				opcode = binary.BigEndian.Uint16(recvBuf[:2])
				block := binary.BigEndian.Uint16(recvBuf[2:4])

				// We have received an ACK.
				if block == blockNumber && opcode == 4 {
					// Making sure to clear out the buffer
					fileBuf = make([]byte, 512)

					blockNumber += 1
					fileN, fileErr := fileFd.Read(fileBuf)
					if fileErr != nil && fileErr != io.EOF {
						fmt.Println("Error reading file:", fileErr)
					}

					msgBuffer = GenerateDataBuffer(
						blockNumber, 03, fileBuf, fileN,
					)

					if sendErr := syscall.Sendto(innerFd, msgBuffer, 0, &clientSa); sendErr != nil {
						fmt.Println("Error sending file: ", sendErr)
					}

					if fileN < 512 {
						syscall.Close(innerFd)
						fmt.Printf("transfer of file %s complete", fileName)
						break
					}

				}
			}
		}
	}
}

func GenerateDataBuffer(
	blockNumber uint16, opcode uint16, msg []byte, msgLen int,
) []byte {
	/*
	 * DATA TFTP PACKET
	 *
	 * 2 bytes    | 2 bytes | n bytes |
	 * --------------------------------
	 *  OPCODE 03 | Block # | Data    |
	 *  -------------------------------
	 */
	buffer := [516]byte{}
	binary.BigEndian.PutUint16(buffer[0:2], opcode)
	binary.BigEndian.PutUint16(buffer[2:4], blockNumber)

	for i := 0; i < msgLen; i++ {
		buffer[i+4] = msg[i]
	}
	return buffer[0 : msgLen+4]
}

func main() {
	args := os.Args[1:]

	if args[0] == "client" {
		runClient()
	} else if args[0] == "server" {
		runServer()
	} else {
		fmt.Println("run `./tftp client` or `./tftp server`")
	}

}
