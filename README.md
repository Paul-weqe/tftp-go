## TFTP-GO

So you want to run tftp using golang do you ?
I have something just for you. Not a complete implementation of the 10 page RFC 
1350 but this can get the job done most of the time.

So how do we run this implementation of ours ?
Follow along:

### Clone the repo

```
git clone https://github.com/Paul-weqe/tftp-go
```

### move into the repo

```
cd tftp-go
```

### Run the service.

TFTP is either run as:

- a client: who wants to get a file from a server.
- a server: who wants to host items that people can get from the client.

For a client, you need to specify the address/port of the server you are 
connecting to, the location on the server to the file you aim to retrieve 
and the mode in which you want the file. In this specific implementation 
however, the mode is not signifcant.

For a server, you need to specify just the port you want to be listening to.

How do you run a client:
```
go run . client
```

How do you run a server:
```
go run . server
```

Both will prompt you for the relevant information. 

There are still a good number of things that are held together by tape here e.g 
we cannot use IPV6 addresses, only text files as of now etc, but will be fixed 
over time. 

Enjoy :)
Or not.
