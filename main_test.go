package main

import (
	"encoding/json"
	"testing"

	"github.com/direktoren/go-nats-go/pkg/easycrypt"
	"github.com/stretchr/testify/assert"
)

func TestByteMessageFunc(t *testing.T) {
	data := []byte("This is the test string that is the bulk of our message")
	generateMessage := byteMessageFunc(data)

	var total uint64 = 10
	var count uint64
	for ; count < total; count++ {
		testRawMessage := generateMessage(count, total)

		assert.Equal(t, string([]byte{0, 0, 0, 0}), testRawMessage.messageType())
		assert.Equal(t, string([]byte{0, 0, 0, 0}), testRawMessage.format())

		message := byteMessage(testRawMessage.message())
		assert.Equal(t, count, message.count())
		assert.Equal(t, total, message.total())
		assert.Equal(t, data, message.data())
	}
}

func TestStructMessageFunc(t *testing.T) {
	data := struct{ MyData string }{"This is the test string that is the bulk of our message"}
	generateMessage := structMessageFunc(&data)

	var total uint64 = 10
	var count uint64
	for ; count < total; count++ {
		testRawMessage := generateMessage(count, total)

		assert.Equal(t, string([]byte{0, 0, 0, 0}), testRawMessage.messageType())
		assert.Equal(t, string([]byte{0, 0, 0, 0}), testRawMessage.format())

		message := testRawMessage.message()
		copyStruct := structMessage{Data: &struct{ MyData string }{}}
		err := json.Unmarshal(message, &copyStruct)

		assert.Equal(t, err, nil, "json.Unmarshal failed")

		assert.Equal(t, count, copyStruct.count())
		assert.Equal(t, total, copyStruct.total())
		assert.Equal(t, &data, copyStruct.Data)

		notTheData := struct{ MyData string }{"Just another message"}
		assert.NotEqual(t, &notTheData, copyStruct.Data)
	}
}

func TestEncryptedMessageFunc(t *testing.T) {
	data := []byte("This is the test string that is the bulk of our message")
	key := "ThisIsMy32BytesKeyForTestingFine"
	generateMessage := encryptedMessageFunc(byteMessageFunc(data), key)

	var total uint64 = 10
	var count uint64
	for ; count < total; count++ {
		testEncryptedRawMessage := generateMessage(count, total)

		assert.Equal(t, string([]byte{0, 0, 0, 0}), testEncryptedRawMessage.messageType())
		assert.Equal(t, string([]byte{0, 0, 0, 0}), testEncryptedRawMessage.format())

		message := testEncryptedRawMessage.message()
		tmpDecrypted, err := easycrypt.Decrypt(message, key)
		assert.Equal(t, err, nil, "Decrypt failed")

		decryptedMessage := byteMessage(tmpDecrypted)
		assert.Equal(t, count, decryptedMessage.count())
		assert.Equal(t, total, decryptedMessage.total())
		assert.Equal(t, data, decryptedMessage.data())
	}
}

func TestRawMessageFunc(t *testing.T) {
	data := []byte("This is the test string that is the bulk of our message")
	msgType := "test"
	format := "byte"
	generateMessage := rawMessageFunc([]byte(msgType), []byte(format), byteMessageFunc(data))

	var total uint64 = 10
	var count uint64
	for ; count < total; count++ {
		testRawMessage := generateMessage(count, total)

		assert.Equal(t, msgType, testRawMessage.messageType())
		assert.Equal(t, format, testRawMessage.format())

		message := byteMessage(testRawMessage.message())
		assert.Equal(t, count, message.count())
		assert.Equal(t, total, message.total())
		assert.Equal(t, data, message.data())
	}
}
