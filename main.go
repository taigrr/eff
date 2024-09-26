package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

func main() {
	pinger, err := probing.NewPinger("127.0.0.1")
	if err != nil {
		panic(err)
	}

	// Ensure IPv4 is used
	pinger.SetNetwork("ip4")

	// Set packet size (optional)
	pinger.Size = 56 // Standard size for ICMP packets
	// Listen for Ctrl-C.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			pinger.Stop()
		}
	}()
	pinger.Interval = time.Second * 3
	pinger.ResolveTimeout = time.Second * 3

	pinger.OnRecv = func(pkt *probing.Packet) {
		fmt.Printf("%d bytes from %s: icmp_seq=%d time=%v\n",
			pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt)
	}
	pinger.OnFinish = func(stats *probing.Statistics) {
		fmt.Printf("\n--- %s ping statistics ---\n", stats.Addr)
		fmt.Printf("%d packets transmitted, %d packets received, %v%% packet loss\n",
			stats.PacketsSent, stats.PacketsRecv, stats.PacketLoss)
		fmt.Printf("round-trip min/avg/max/stddev = %v/%v/%v/%v\n",
			stats.MinRtt, stats.AvgRtt, stats.MaxRtt, stats.StdDevRtt)
	}
	pinger.OnSendError = func(_ *probing.Packet, err error) {
		fmt.Printf("Ping send failed: %v\n", err)
	}
	pinger.OnRecvError = func(err error) {
		if neterr, ok := err.(*net.OpError); ok {
			if neterr.Timeout() {
				return
			}
		}

		fmt.Printf("Ping recv failed: %v\n", err)
	}
	fmt.Printf("PING %s (%s):\n", pinger.Addr(), pinger.IPAddr())
	err = pinger.Run()
	if err != nil {
		panic(err)
	}
}
