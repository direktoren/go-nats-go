# go-nats-go
Performance simulation/test of goprog -> NATS -> goprog

## Purpose ##
**go-nats-go** simulates/tests the time from *START* when the data is available in one go program -> nats -> to *STOP* when the data it is available for use in another go program.

Example (json/encr):

1. go-nats-go slave subscribes to subject@nats
2. go-nats-go master has *N* structs with data
3. Timestamp *START*
4. go-nats-go master marshals the *N* struct into json
5. go-nats-go master encrypts the *N* json
6. go-nats-go master publishes *N* messages on Subject@nats
7. nats does it's thing
8. go-nats-go slave receives *N* messages
9. go-nats-go slave decrypts the *N* messages
10. go-nats-go slave unmarshals the *N* json
11. Timestamp *STOP*
 
Example (byte/byte):

1. go-nats-go slave subscribes to subject@nats
2. go-nats-go master has *N* []byte slices
3. Timestamp *START*
4. go-nats-go master publishes *N* messages on subject@nats
5. nats does it's thing
6. go-nats-go slave receives *N* messages
7. Timestamp *STOP*

Other:
- byte/byte byte/encr json/byte json/encry
- byte can be filled 0 bytes or pre-loaded from file.
- Feel free to update with more scenarios!

## Outcome ##

**go-nats-go** produces (if successful)
- Total Duration (*STOP* - *START*)
- Average Duration (*STOP* - *START*) / *N*

# Usage

### Install ###
Install using

```
> go install github.com/direktoren/go-nats-go
```

### Configure ###
Make sure nats-io is up and running. Select two servers with connection to yout nats, or run from the same server. The program needs to be executed from both servers, one as master and one as slave. No more. No less. Update the config.json file to point to your nats. If you needs different nats-url for master and slave - just set that in each config.json. If you want to change settings, number of messages scenario etc - update the config file.

Config file example:

```
{
    "Subject": "go-nats-go", 
    "Total": 10000,
    "NATSServerURL": "nats://localhost:4222",  
    "Timeout": 1000000000000,
    "Scenario": "emptybytes",
    "AESEncryptionKey": "DontUseARealKey.ItHasToBe32Bytes",
    "NumBytes": 16000,
    "Filename": "bytes.txt"
}
```

Scenarios:

`"json"`
Just marshal a struct to json

`"json.encrypted"`
Marshal a struct to json and then encrypt using *AESEncryptionKey*

`"emptybytes"`
Create *NumBytes* empty bytes payload

`"file"`
Populate message once with bytes from *Filename* 

`"file.encrypted"`
Populate message once with bytes from *Filename* and then encrypt using *AESEncryptionKey*

### Run ###
Start the slave first with `-s` option

```
> go-nats-go -o config.json -s
INFO[0000] Starting to do the work as slave=true.
```

Then run the master in a different terminal and/or on a different box

```
> go-nats-go -o config.json
INFO[0000] Starting to do the work as slave=false.
INFO[0000] All messages sent & summary message received.
INFO[0000] Mode=json/encr
INFO[0000] Message size=1154 (byte)
INFO[0000] Message generation=19.12µs
INFO[0000] Total duration=120.917ms
INFO[0000] Total Messages=1000
INFO[0000] Duration/Message=120.917µs
INFO[0000] Closing down.
```

And you get output from the slave

```
INFO[0008] Accepted a new job with Total=1000
INFO[0008] Completed a job with Total=1000
```

Slave will remain alive ready to handle more jobs until Ctrl-c or after *Timeout* specified in the config.json. You don't have to restart the slave if you are testing different scenarios. But you need to restart the slave if you have changed *Subject*, *NATSServerURL* or *AESEncryptionKey*.

Master will close after the job is finished, Ctrl-c or *Timeout*. Advice - unless slave confirms a new job - something probably went wrong.

### Note: ####
 - regarding the encryption key: Of course we would never store an encryption key in plain text in a config file for production app. But since we are only testing the mechanism just set any 32-byte key BUT use the same for master and slave.
 - Metrics values assume clocks are in sync on where go-nats-go master and go-nats-go slave is running.

### TODO: ###
- Add support for nats credential, tls etc
