package sijsop

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
)

type FileTransfer struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func (ft *FileTransfer) SijsopType() string {
	return "file_transfer"
}

func (ft *FileTransfer) New() Message {
	return &FileTransfer{}
}

type FileTransferAck struct {
	Name string `json:"name"`
}

func (fta *FileTransferAck) SijsopType() string {
	return "file_transfer_ack"
}

func (fta *FileTransferAck) New() Message {
	return &FileTransferAck{}
}

type RW struct {
	io.ReadCloser
	io.WriteCloser
}

// This example demonstrates transferring a binary file using the sijsop
// protocol, with the file transferred out-of-band. Left is going to read a
// file and send it to Right, who will acknowledge it.
func Example() {
	toRightRead, fromLeftWrite := io.Pipe()
	toLeftRead, fromRightWrite := io.Pipe()

	protocol := &Definition{}
	protocol.Register(&FileTransfer{}, &FileTransferAck{})

	wg := sync.WaitGroup{}
	wg.Add(2)

	// The left side, who will be reading the file and sending it
	go func() {
		sp := protocol.Wrap(RW{toLeftRead, fromLeftWrite})

		f, err := os.Open("sijsop_example_test.go")
		if err != nil {
			panic("Couldn't read this file")
		}

		fi, err := f.Stat()
		if err != nil {
			panic("couldn't stat this file")
		}

		transfer := &FileTransfer{
			Name: "sijsop_example_test.go",
			Size: fi.Size(),
		}

		err = sp.Send(transfer)
		if err != nil {
			panic("Couldn't send transfer request")
		}

		// now we copy the file over the stream directly
		io.Copy(fromLeftWrite, f)

		ack := &FileTransferAck{}
		err = sp.Unmarshal(ack)
		if err != nil {
			panic("didn't get the expected ack")
		}

		// deliberately done here, to ensure we received the ack
		wg.Done()
	}()

	// The right side, who will be receiving it and ack'ing it.
	go func() {
		sp := protocol.Wrap(RW{toRightRead, fromRightWrite})

		// show just receiving next message
		msg, err := sp.ReceiveNext()
		if err != nil {
			panic("couldn't get the file transfer request")
		}

		size := msg.(*FileTransfer).Size

		// Now directly extract the file from the bytestream
		r := &io.LimitedReader{toRightRead, size}
		buf := &bytes.Buffer{}
		io.Copy(buf, r)

		// and send the ack, transititioning back to JSON
		err = sp.Send(&FileTransferAck{msg.(*FileTransfer).Name})
		if err != nil {
			panic("couldn't send acknowledgement")
		}

		fileContents := buf.String()
		// print out the package declaration to show the file was truly
		// transferred.
		fmt.Println(fileContents[:15])

		wg.Done()
	}()

	wg.Wait()

	// Output:
	// package sijsop
}
