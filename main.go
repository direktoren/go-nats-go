package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"time"

	"github.com/dikektoren/go-nats-go/pkg/easycrypt"

	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/tkanos/gonfig"
)

/* --------------------- CONFIGURATION --------------------- */

type configuration struct {
	Subject       string
	Total         uint64
	NATSServerURL string
	Timeout       time.Duration

	Scenario         string
	AESEncryptionKey string

	NumBytes uint
	Filename string
}

func readConfig(fileName string, config *configuration) error {
	err := gonfig.GetConf(fileName, config)
	if err != nil {
		return errors.Wrap(err, "config: gonfig.getconf issue")
	}

	// Now verify some of the configs
	if len(config.AESEncryptionKey) != 32 {
		return errors.New("config: len(config.AESEncryptionKey) != 32")
	}

	if config.Subject == "" {
		config.Subject = "speedtestnats"
	}

	if config.NATSServerURL == "" {
		config.NATSServerURL = nats.DefaultURL
	}

	return nil
}

/* --------------------- BYTE MESSAGE STRUCTURE  ---------------------

			Type		Format			Message
			[4]byte		[4]byte			[]byte

Type									Count				Total				Data
			"byte"					-->	[8]byte (uint64)	[8]byte (uint64)	[]byte

			"json"					-->	Message.Count		Message.Total		Message.Data (interface{})
										Struct marshalled into json message ([]byte)


Format
						"byte"		--> Raw []byte data for Message

						"encr"		--> Encrypted []byte with AES 32 byte key

*/

type rawMessage []byte

func (raw rawMessage) messageType() string {
	return string(raw[:4])
}

func (raw rawMessage) format() string {
	return string(raw[4:8])
}

func (raw rawMessage) message() []byte {
	return raw[8:]
}

type message interface {
	count() uint64
	total() uint64
}

// byteMessage
type byteMessage []byte

func (bytes byteMessage) count() uint64 {
	value, _ := binary.Uvarint(bytes[:8])
	return value
}

func (bytes byteMessage) total() uint64 {
	value, _ := binary.Uvarint(bytes[8:16])
	return value
}

func (bytes byteMessage) data() []byte {
	return bytes[16:]
}

// structMessage
type structMessage struct {
	Count uint64
	Total uint64
	Data  interface{}
}

func (msg structMessage) count() uint64 {
	return msg.Count
}

func (msg structMessage) total() uint64 {
	return msg.Total
}

/* --------------------- MESSAGE FUNCS --------------------- */

// Type for functions that generates a raw message with data on current count and total
type rawMessageGenerator func(uint64, uint64) rawMessage

// Most basic rawMessage generator. copies the data to a new message and adds metadata bytes
func byteMessageFunc(data []byte) rawMessageGenerator {
	return func(count uint64, total uint64) rawMessage {
		msg := make(rawMessage, 4+4+8+8+len(data))
		binary.PutUvarint(msg[8:16], count)  // Add count
		binary.PutUvarint(msg[16:24], total) // Add total
		copy(msg[24:], data)                 // Copy the date to byte 24+
		return msg
	}
}

// rawMessage generator for structs, using json.Marshal. No error handling
func structMessageFunc(v interface{}) rawMessageGenerator {
	return func(count uint64, total uint64) rawMessage {
		myStruct := structMessage{count, total, v} // Adds count, total and the v struct data
		msgBody, _ := json.Marshal(&myStruct)
		msg := make(rawMessage, 4+4+len(msgBody))
		copy(msg[8:], msgBody)
		return msg
	}
}

// Takes a rawMessage generator and wraps with encryption based on aes key
func encryptedMessageFunc(generateMessage rawMessageGenerator, key string) rawMessageGenerator {
	return func(count uint64, total uint64) rawMessage {
		msg := generateMessage(count, total)
		encryptedBody, _ := easycrypt.Encrypt(string(msg[8:]), key)
		encryptedMessage := make(rawMessage, 8+len(encryptedBody))
		copy(encryptedMessage[8:], encryptedBody)
		return encryptedMessage
	}
}

// Wraps rawmessage generators and sets the final msgType and format bytes
func rawMessageFunc(msgType []byte, format []byte, generateMessage rawMessageGenerator) rawMessageGenerator {
	return func(count uint64, total uint64) rawMessage {
		msg := generateMessage(count, total)
		copy(msg[:4], msgType) // Add MsgType
		copy(msg[4:8], format) // Add format
		return msg
	}

}

/* --- */

// Just a simple struct to use in json tests
type bigStruct struct {
	Name string

	Pets []struct {
		Bites   bool
		CanFly  bool
		Ignores string
	}

	LastGolfScores []int

	Points float64
	Games  []struct {
		Against       string
		Fun           bool
		MinutesPlayed float64
	}
}

func fillBigStruct() bigStruct {
	return bigStruct{Name: "Steve Rogers",
		Pets: []struct {
			Bites   bool
			CanFly  bool
			Ignores string
		}{{true, true, "Polly"}, {false, false, "Nothing"}, {true, false, "Cat/MrCat/*"}, {false, false, "Turtle"},
			{false, true, "Parrot2"}, {true, false, "Leave my backyard!"}},
		LastGolfScores: []int{83, 87, 89, 104, 90, 113, 104, 88, 88, 98, 79, 97, 120, 110},
		Points:         345.32,
		Games: []struct {
			Against       string
			Fun           bool
			MinutesPlayed float64
		}{{"Stoke", false, 30.2}, {"Flyfield", false, 60.4}, {"Figgerish", false, 73.4}, {"Tomland", true, 30.4},
			{"Huddersfield", true, 33.12}, {"Fulham", true, 13.112}, {"Brentford", false, 94}, {"Magneto", false, 1000.4},
			{"Mom", true, 90.4}, {"Sis", true, 45.2}, {"Pop", true, 89.2}, {"Brother", false, 10.4}}}
}

/* --------------------- MAIN --------------------- */

// metrics is the struct for the message to communicate time spend between master & slave
type metric struct {
	Job   string
	Time  time.Time
	Count uint64
}

func main() {
	log := logrus.New()
	log.Out = os.Stderr

	// Select flag options and parse
	var configFile string
	var slave bool
	flag.StringVar(&configFile, "o", "config.json", fmt.Sprintf("Set name and path to config file"))
	flag.BoolVar(&slave, "s", false, fmt.Sprintf("Set to run as slave"))
	flag.Parse()

	// Get & Set configs & global vards
	var config = configuration{}
	err := readConfig(configFile, &config)
	if err != nil {
		log.Logf(logrus.FatalLevel, "readConfig issue err=%v", err)
		return
	}

	// Create context & waitgroup & nats connection
	ctx, cancelFunction := context.WithTimeout(context.Background(), config.Timeout)
	defer cancelFunction()
	//var wg sync.WaitGroup

	log.Logf(logrus.InfoLevel, "Starting to do the work as slave=%v.", slave)
	defer log.Logf(logrus.InfoLevel, "Closing down.")

	nc, err := nats.Connect(config.NATSServerURL)
	defer nc.Close()

	var generateMessageFunction rawMessageGenerator

	/* ------------- ADD YOUR OWN SCENARIOS HERE ------------- */

	switch config.Scenario {

	case "json":

		// Message based on Marshal the bigStruct
		myStruct := fillBigStruct()
		generateMessageFunction = rawMessageFunc([]byte("json"), []byte("byte"), structMessageFunc(&myStruct))

	case "json.encrypted":

		// Message based on encrypted Marshal of the bigStruct
		myStruct := fillBigStruct()
		generateMessageFunction = rawMessageFunc([]byte("json"), []byte("encr"), encryptedMessageFunc(structMessageFunc(&myStruct), config.AESEncryptionKey))

	case "emptybytes":

		// Messages with config.Numbytes empty zeros
		data := make([]byte, config.NumBytes)
		generateMessageFunction = rawMessageFunc([]byte("byte"), []byte("byte"), byteMessageFunc(data))

	case "file":

		// Messages created from config.Filename. No error handling. Note: file data is copied in memory for message generation
		// Not read from disk except this first time
		data, err := ioutil.ReadFile(config.Filename)
		if err != nil {
			log.Logf(logrus.FatalLevel, "Unable to read file err=%v", err)
			return
		}
		generateMessageFunction = rawMessageFunc([]byte("byte"), []byte("byte"), byteMessageFunc(data))

	case "file.encrypted":

		// Encrypted file data
		data, err := ioutil.ReadFile(config.Filename)
		if err != nil {
			log.Logf(logrus.FatalLevel, "Unable to read file err=%v", err)
			return
		}
		generateMessageFunction = rawMessageFunc([]byte("byte"), []byte("encr"), encryptedMessageFunc(byteMessageFunc(data), config.AESEncryptionKey))

	}

	/* ---------------------- SERVICES ----------------------*/

	totalDuration := -1 * time.Second
	fc := make(chan struct{})

	switch slave {
	case false:

		// We are the master. Store the first 'base' time stamp
		base := metric{"base", time.Now(), config.Total}

		// Fire away the config.Total number of messages on subject config.Subject+".data"
		go func(ctx context.Context, nc *nats.Conn, subject string, generateMessage rawMessageGenerator) {
			var count uint64
			for ; count < config.Total; count++ {
				msg := generateMessage(count, config.Total)
				nc.Publish(subject, []byte(msg))
			}

		}(ctx, nc, config.Subject+".data", generateMessageFunction)

		// Service that listens to the .metric subject to get timestamp back from the slave
		nc.Subscribe(config.Subject+".metric", func(msg *nats.Msg) {
			m := metric{}
			json.Unmarshal(msg.Data, &m)
			if m.Job == "received" && m.Count == config.Total {
				totalDuration = m.Time.Sub(base.Time)

				// Signal that we are done
				fc <- struct{}{}
			}
		})

	case true:

		// We found ourselves to be slave...
		// We listen to the .data subject
		// Send back timestamp when we have received Total amount of messages and we started with Count 0 and ended with Count == Total-1
		// Succesful decrypt is required before sending back timestamp. But limited message verification
		// If times are not in sync between master and slave then the message/duration times will be wrong
		var receivedCounter uint64
		nc.Subscribe(config.Subject+".data", func(msg *nats.Msg) {
			defer func() { receivedCounter++ }()

			// First decrypt the "message body"
			msgBytes := rawMessage(msg.Data).message()
			switch rawMessage(msg.Data).format() {
			case "encr":
				msgBytes, err = easycrypt.Decrypt(msgBytes, config.AESEncryptionKey)
				if err != nil {
					// Ignore messages that cannot be decrypted
					return
				}
			case "byte":
			}

			var receivedMessage message
			switch rawMessage(msg.Data).messageType() {
			case "byte":
				receivedMessage = byteMessage(msgBytes)
			case "json":
				tmpStruct := structMessage{}
				err := json.Unmarshal(msgBytes, &tmpStruct)
				if err != nil {
					// Ignore messages that cannot be unmarshalled
					return
				}
				receivedMessage = tmpStruct
			default:
				receivedMessage = byteMessage(msgBytes)
			}

			if receivedMessage.total() == 0 {
				return // Ignore messages with total==0
			}

			if receivedMessage.count() == 0 {
				receivedCounter = 0 // First message in the "stream". We have a new job!
				log.Logf(logrus.InfoLevel, "Accepted a new job with Total=%d", receivedMessage.total())
			}

			if receivedMessage.count() == receivedMessage.total()-1 && receivedCounter == receivedMessage.total()-1 {
				// Send back metrics when received and message with right count is received
				bytes, _ := json.Marshal(&metric{"received", time.Now(), receivedMessage.total()})
				nc.Publish(config.Subject+".metric", bytes)
				log.Logf(logrus.InfoLevel, "Completed a job with Total=%d", receivedMessage.total())
			}

		})

	}

	/* ---------------------- END SERVICES ----------------------*/

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	select {
	case <-fc: // Work is done - we have received confirmation back from the slave

		beforeMessageTime := time.Now()
		testMessage := generateMessageFunction(1, 1)
		afterMessageTime := time.Now()
		msgDuration := afterMessageTime.Sub(beforeMessageTime)

		// Compile a short summary of the outcome
		log.Logf(logrus.InfoLevel, "All messages sent & summary message received.")
		log.Logf(logrus.InfoLevel, "Mode=%s/%s", testMessage.messageType(), testMessage.format())
		log.Logf(logrus.InfoLevel, "Message size=%d (byte)", len(testMessage))
		log.Logf(logrus.InfoLevel, "Message generation=%v", msgDuration)
		log.Logf(logrus.InfoLevel, "Total duration=%v", totalDuration)
		log.Logf(logrus.InfoLevel, "Total Messages=%d", config.Total)
		log.Logf(logrus.InfoLevel, "Duration/Message=%v", totalDuration/(time.Duration)(config.Total))

	case <-ctx.Done(): // Context expired. Likely timeout

		log.Logf(logrus.InfoLevel, "Timeout! For longer timeout - Change the settings in config file!")

	case <-c: // User interrupt

		log.Logf(logrus.InfoLevel, "User abort.")
	}

	nc.Flush()
	nc.Drain()
}
