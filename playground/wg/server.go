package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

func main() {
	log.Println("Starting WireGuard server (192.168.4.2)...")

	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("192.168.4.2")},
		[]netip.Addr{netip.MustParseAddr("8.8.8.8")},
		1420,
	)
	if err != nil {
		log.Panic(err)
	}

	dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))
	err = dev.IpcSet(`private_key=003ed5d73b55806c30de3f8a7bdab38af13539220533055e635690b8b87ad641
listen_port=51822
public_key=f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c
allowed_ip=192.168.4.1/32
persistent_keepalive_interval=25
`)
	if err != nil {
		log.Panic(err)
	}
	err = dev.Up()
	if err != nil {
		log.Panic(err)
	}

	log.Println("WireGuard interface up. Starting HTTP server on :80...")

	listener, err := tnet.ListenTCP(&net.TCPAddr{Port: 80})
	if err != nil {
		log.Panicln(err)
	}

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		log.Printf("> %s - %s - %s", request.RemoteAddr, request.URL.String(), request.UserAgent())
		io.WriteString(writer, "pong from userspace WireGuard!")
	})

	log.Println("Server ready. Listening on 192.168.4.2:80")
	err = http.Serve(listener, nil)
	if err != nil {
		log.Panicln(err)
	}
}
