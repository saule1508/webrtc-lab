# Test webrtc connections with golang PION library

this program is inspired by examples in the https://github.com/pion/webrtc

In order to have peer to peer connection with Webrtc you need a server to do the signalling. This is needed because each peer needs to exchange connection information with the other before the connection is established. Furthermore you need STUN servers in order to discover your public IP and punch holes in the NAT (STUN is Traversal utilities arount NAT)

The signaling server is in the other project (https://gitlab.global.ingenico.com/usrvpn/code/ice-test/signaling-server)

There are a lot of documentation on webrtc on internet, a good reference is  https://developer.mozilla.org/en-US/docs/Web/API/WebRTC_API. Webrtc is essentially a javascript in the browser framework but there is a good golang implementation (PION). Also webrtc requires server (signaling and stun/turn) and for that golang is very good (https://www.youtube.com/watch?v=nRZePB4kzWo)

# how to use

## First: start a signaling.

the signaling server must be accessible for both peers. If you want to test from a zscaler you have to let the signaling server listen on 443 (otherwise zscaler will block)

I use nbrpeersb0101a1.usrvpn.sb.au.ginfra.net because it has a public IP

to have a binary, clone the project and then
```
make build
```
the binary is in the bin directory

```
scp bin/signaling-server ptimmerman@nbrpeersb0101a1.usrvpn.sb.au.ginfra.net:~
```

before starting, make sure port 443 is open and disable pat so it is not closed by puppet.

```
pat ds test webrtc
sudo iptables -I INPUT -p tcp --dport 443 -j ACCEPT
/home/users/ptimmerman/signaling-server -port 443
```


## Second: webrtc answer client

you can have this peer on any server you want to test, it needs to reach the signaling server of course.

you can copy a binary or run the go program 

```
go run webrtc_client.go -role answerer -signaling-addr http://160.92.91.178:443 -with-turn=false
```

it will register to the signaling server via http, and wait on the signaling for message sent by the other peer. 

When it receives an offer from the other peer, it will reply with an answer and if possible the peer to peer connection will be opened. Then they will exchange messages. At the core of webrtc, the two peers continuously exchange ICE candidates, so that they can find the most efficient way to connect. ICE candidates gathering is using STUN as a way to discover the public IP and various techniques to traverse NAT (by punching holes in the firewall)

to have a binary, clone the project and then
```
make build
```
the binary is in the bin directory


## Third: webrtc offer client

copy the binary or the go source to the peer

```
go run webrtc_client.go -role offerer -signaling-addr http://160.92.91.178:443 -with-turn=false
```

It will make an offer (SDP) and send it via signaling to the other peer, then wait for the answer.

the ICE candidate generated are printed and once the connection is established they will exchange messages via a data channel. The info printed helps to understand the webrct protocol, especially the ICE gathering and exchange.  