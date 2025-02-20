package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/pion/webrtc/v3"
)

const offerPeerID = "offer-peer-1"
const answerPeerID = "answer-peer-1"

type SignalingMessage struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	To      string `json:"to"`
	From    string `json:"from"`
}

var clientID = offerPeerID
var remoteSignal = make(chan SignalingMessage)
var logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true, Level: slog.LevelInfo}))
var signalingServer *string

func main() {
	// This is the offer or answer application. Answer must be started first
	role := flag.String("role", "", "Role of the application (offerer or answerer)")
	withTurn := flag.Bool("with-turn", true, "Use TURN server")
	signalingServer = flag.String("signaling-addr", "http://127.0.0.1:8080", "Signaling server address")
	var turnPassword string
	var candidateReceivedBeforeOfferOrAnswer = []*webrtc.ICECandidateInit{}
	var offerReceived = false
	var answerReceived = false
	var localCandidates []string
	var remoteCandidates []string

	flag.Parse()
	if *role == "answerer" {
		clientID = answerPeerID
	} else if *role == "offerer" {
		clientID = offerPeerID
	} else {
		logger.Error("Invalid role", "role", *role)
		os.Exit(1)
	}
	// Establish connection to signaling server
	go listenForSignaling(clientID)
	if *withTurn {
		turnPassword := os.Getenv("TURN_PASSWORD")
		if turnPassword == "" {
			logger.Error("TURN_PASSWORD environment variable is not set")
			os.Exit(1)
		}
	}
	iceServersURL := []string{
		"stun:turn1.usrvpn.sandbox.apac.giservices.io:443",
	}
	if *withTurn {
		logger.Info("Using TURN")
		iceServersURL = append(iceServersURL, "turns:turn1.usrvpn.sandbox.apac.giservices.io:443?transport=tcp")
	} else {
		logger.Info("Not using TURN")
	}
	// Create a new WebRTC API instance
	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs:       iceServersURL,
				Username:   "self",
				Credential: turnPassword, // Password for TURN server
			},
		},
	})

	if err != nil {
		logger.Error("Failed to create peer connection", slog.String("error", err.Error()))
		os.Exit(1)
	}
	// Create a data channel
	if *role == "offerer" {
		dataChannel, err := peerConnection.CreateDataChannel("testChannel", nil)
		if err != nil {
			logger.Error("Failed to create data channel", slog.String("error", err.Error()))
			os.Exit(1)
		}

		dataChannel.OnOpen(func() {
			// This is called when the DataChannel is open and ready to send messages.
			logger.Info("DataChannel is open, you can send messages now.")

			message := []byte("Hello from the Offerer!")
			err := dataChannel.Send(message) // Send a message
			if err != nil {
				logger.Error("Error sending message", slog.String("error", err.Error()))
			} else {
				logger.Info("Message sent")
			}
			ticker := time.NewTicker(20 * time.Second)
			go func() {
				defer ticker.Stop()
				for t := range ticker.C {
					if dataChannel.ReadyState() == webrtc.DataChannelStateOpen {
						message := []byte(fmt.Sprintf("Hello from the Offerer at %s!", t.Format(time.RFC3339)))
						err := dataChannel.Send(message)
						if err != nil {
							logger.Error("Error sending message", slog.String("error", err.Error()))
						} else {
							logger.Info("Message sent")
						}
					} else {
						logger.Warn("DataChannel is not open, unable to send message")
						break // Break the loop if DataChannel is not open
					}
				}
			}()
		})
		dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
			logger.Info("Message received on data channel:", "data", string(msg.Data))
			if *role == "answerer" {
				message := []byte("This is my reply from the Answerer!")
				err := dataChannel.Send(message) // Send a message
				if err != nil {
					logger.Error("Error sending message", slog.String("error", err.Error()))
				} else {
					logger.Info("Message sent")
				}
			}
		})
	}
	if *role == "answerer" {
		peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
			dataChannel := d // Store the received data channel

			dataChannel.OnOpen(func() {
				logger.Info("DataChannel is open on answerer, you can send messages now.")
				// Answerer can now send messages too if needed.
				message := []byte("Hello from the Answerer!")
				err := dataChannel.Send(message) // Send a message
				if err != nil {
					logger.Error("Error sending message", slog.String("error", err.Error()))
				} else {
					logger.Info("Message sent")
				}
			})

			dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
				logger.Info("Message received on data channel:", "data", string(msg.Data))
			})
		})
	}
	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		logger.Info("Peer connection state changed", "state", state.String())
		for _, c := range localCandidates {
			logger.Info("local candidate", "candidate", c)
		}
		for _, c := range remoteCandidates {
			logger.Info("remote candidate", "candidate", c)
		}
		// this does not work well, info retrieved is not good enough
		// printConnectionStats(peerConnection)
	})

	// Handle incoming ICE candidates
	peerConnection.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i != nil {
			var sendTo string
			if *role == "offerer" {
				sendTo = answerPeerID
			} else {
				sendTo = offerPeerID
			}
			localCandidates = append(localCandidates, i.ToJSON().Candidate)
			candidateJSON := i.ToJSON()
			logger.Info("local ICE Candidate", "type", i.Typ, "address", i.Address, "tcptype", i.TCPType, "port", i.Port, "priority", i.Priority,
				"protocol", i.Protocol, "related_addr", i.RelatedAddress, "related_port", i.RelatedPort, "ID", candidateJSON.SDPMid)
			logger.Debug("Sending ICE Candidate to", "ToPeer", sendTo)
			sendICECandidate(candidateJSON.Candidate, sendTo)
		}
	})
	if *role == "offerer" {
		logger.Info("the offerer is going to create an offer and set local description")
		offer, err := peerConnection.CreateOffer(nil)
		if err != nil {
			logger.Error("Failed to create offer", slog.String("error", err.Error()))
			os.Exit(1)
		}
		err = peerConnection.SetLocalDescription(offer)
		if err != nil {
			logger.Error("Failed to set local description", slog.String("error", err.Error()))
			os.Exit(1)
		}

		// Check if the SDP is not empty before sending
		if offer.SDP == "" {
			logger.Error("The offer SDP is empty. Unable to send to signaling server.")
			os.Exit(1)
		}
		// Send the offer to the remote peer through the signaling server
		logger.Info("Sending offer", "ToPeer", answerPeerID, "offer", offer.SDP)
		sendSignal(offer.SDP, "offer", clientID, answerPeerID)

		for _, candidate := range candidateReceivedBeforeOfferOrAnswer {
			err = peerConnection.AddICECandidate(*candidate)
			if err != nil {
				logger.Error("Failed to add ICE candidate", slog.String("error", err.Error()))
				os.Exit(1)
			}
		}
	}
	for {
		answer := <-remoteSignal
		log.Printf("Received message of type %s : %s\n", answer.Type, answer.Content)
		if answer.Type == "offer" && *role == "answerer" { // TODO: role will always be answerer in fact
			err := peerConnection.SetRemoteDescription(webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  answer.Content,
			})
			if err != nil {
				logger.Error("Failed to set remote description", slog.String("error", err.Error()))
				os.Exit(1)
			}
			answer, err := peerConnection.CreateAnswer(nil)
			if err != nil {
				logger.Error("Failed to create answer", slog.String("error", err.Error()))
				os.Exit(1)
			}

			// Set the local description to the answer created
			err = peerConnection.SetLocalDescription(answer)
			if err != nil {
				logger.Error("Failed to set local description", slog.String("error", err.Error()))
				os.Exit(1)
			}
			sendSignal(answer.SDP, "answer", clientID, offerPeerID)
			for _, candidate := range candidateReceivedBeforeOfferOrAnswer {
				err = peerConnection.AddICECandidate(*candidate)
				if err != nil {
					logger.Error("Failed to add ICE candidate", slog.String("error", err.Error()))
					os.Exit(1)
				}
			}
			offerReceived = true
		} else if answer.Type == "candidate" && *role == "answerer" && !offerReceived {
			logger.Info("Received a candidate before the offer", "content", answer.Content)
			// Parse the candidate
			iceCandidate := webrtc.ICECandidateInit{
				Candidate: answer.Content,
			}
			candidateReceivedBeforeOfferOrAnswer = append(candidateReceivedBeforeOfferOrAnswer, &iceCandidate)
			remoteCandidates = append(remoteCandidates, answer.Content)

		} else if answer.Type == "answer" && *role == "offerer" { // TODO: role will always be offerer in fact
			remoteDesc := webrtc.SessionDescription{
				Type: webrtc.SDPTypeAnswer,
				SDP:  answer.Content,
			}
			err := peerConnection.SetRemoteDescription(remoteDesc)
			if err != nil {
				logger.Error("Failed to set remote description", slog.String("error", err.Error()))
				os.Exit(1)
			}
			answerReceived = true
		} else if answer.Type == "candidate" && *role == "offerer" && !answerReceived {
			logger.Info("Received a candidate before the answer", "content", answer.Content)
			// Parse the candidate
			iceCandidate := webrtc.ICECandidateInit{
				Candidate: answer.Content,
			}
			candidateReceivedBeforeOfferOrAnswer = append(candidateReceivedBeforeOfferOrAnswer, &iceCandidate)
			remoteCandidates = append(remoteCandidates, answer.Content)
		} else if answer.Type == "candidate" {
			logger.Info("Received a candidate", "content", answer.Content)
			// Parse the candidate
			iceCandidate := webrtc.ICECandidateInit{
				Candidate: answer.Content,
			}
			// Add the candidate to the peer connection
			err := peerConnection.AddICECandidate(iceCandidate)
			if err != nil {
				log.Fatalf("Failed to add ICE candidate: %v", err)
			}
			remoteCandidates = append(remoteCandidates, answer.Content)
		}
	}
}

func sendSignal(content string, typeMsg string, fromClientID string, toClientID string) {
	msg := SignalingMessage{
		Type:    typeMsg,
		Content: content,
		To:      toClientID,
		From:    fromClientID,
	}

	body, err := json.Marshal(msg)
	if err != nil {
		logger.Error("Failed to marshal signal message", slog.String("error", err.Error()))
		os.Exit(1)
	}

	resp, err := http.Post(fmt.Sprintf("%s/send", *signalingServer), "application/json", bytes.NewBuffer(body))
	if err != nil {
		logger.Error("Failed to send signal message", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logger.Error("Failed to send signal message", slog.Int("code", resp.StatusCode))
		os.Exit(1)
	}
}

func sendICECandidate(c string, toClientID string) {
	msg := SignalingMessage{
		Type:    "candidate",
		Content: c,
		To:      toClientID,
		From:    clientID,
	}

	body, err := json.Marshal(msg)
	if err != nil {
		log.Fatalf("Failed to marshal ICE candidate: %v", err)
	}
	resp, err := http.Post(fmt.Sprintf("%s/send", *signalingServer), "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Fatalf("Failed to send ICE candidate: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("Failed to send ICE candidate: %s", resp.Status)
	}
}

func listenForSignaling(clientID string) {
	logger.Info("Listening for signaling messages", "clientID", clientID, "addr", *signalingServer)
	url := fmt.Sprintf("%s/receive?id=%s", *signalingServer, clientID)
	resp, err := http.Get(url)
	if err != nil {
		log.Fatalf("Failed to connect to signaling server: %v", err)
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	for {
		var msg SignalingMessage
		if err := decoder.Decode(&msg); err != nil {
			logger.Error("Failed to decode signaling message", slog.String("error", err.Error()))
			break
		}
		// Handle incoming signaling message (offer/answer)
		logger.Info("Received message", "type", msg.Type, "from", msg.From, "to", msg.To, "content", msg.Content)
		// put the received message in the channel
		if msg.Type != "heartbeat" {
			remoteSignal <- msg
		} else {
			logger.Info("Received heartbeat", "from", msg.From, "to", msg.To, "content", msg.Content)
		}
	}
}

func printConnectionStats(peerConnection *webrtc.PeerConnection) {
	// GetStats will return stats after a short time, we might need to wait
	time.Sleep(2 * time.Second)

	// Retrieve statistics
	stats := peerConnection.GetStats()
	if stats != nil {
		for _, report := range stats {

			if icePairStats, ok := report.(webrtc.ICECandidatePairStats); ok {
				logger.Info("ICE Candidate Pair Stats",
					"localID", icePairStats.LocalCandidateID,
					"remoteID", icePairStats.RemoteCandidateID,
					"state", icePairStats.State,
					"received", icePairStats.BytesReceived,
					"sent", icePairStats.BytesSent)
			}

			// Check for SCTP Transport Stats
			if sctpStats, ok := report.(webrtc.SCTPTransportStats); ok {
				logger.Info("SCTP Transport Stats",
					"type", sctpStats.Type,
					"receive", sctpStats.BytesReceived,
					"sent", sctpStats.BytesSent)
			}
		}
	} else {
		log.Printf("Failed to get stats")
	}
}
