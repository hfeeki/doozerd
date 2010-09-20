package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	"junta/paxos"
	"junta/mon"
	"junta/store"
	"junta/util"
	"junta/client"
	"junta/server"
)

const (
	alpha = 50
	idBits = 160
)


// Flags
var (
	listenAddr *string = flag.String("l", "", "The address to bind to. Must correspond to a single public interface.")
	publishAddr *string = flag.String("p", "", "Address to publish in junta for client connections.")
	attachAddr *string = flag.String("a", "", "The address of another node to attach to.")
)

func activate(st *store.Store, self, prefix string, c *client.Client) {
	logger := util.NewLogger("activate")
	ch := make(chan store.Event)
	st.Watch("/junta/slot/*", ch)
	for ev := range ch {
		// TODO ev.IsEmpty()
		if ev.IsSet() && ev.Body == "" {
			_, err := c.Set(prefix+ev.Path, self, ev.Cas)
			if err == nil {
				return
			}
			logger.Log(err)
		}
	}
}

func Usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] <cluster-name>\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	flag.PrintDefaults()
}

func main() {
	util.LogWriter = os.Stderr
	logger := util.NewLogger("main")

	flag.Parse()
	flag.Usage = Usage

	if len(flag.Args()) < 1 {
		logger.Log("require a cluster name")
		flag.Usage()
		os.Exit(1)
	}

	clusterName := flag.Arg(0)
	prefix := "/j/" + clusterName

	if *listenAddr == "" {
		logger.Log("require a listen address")
		flag.Usage()
		os.Exit(1)
	}

	if *publishAddr == "" {
		*publishAddr = *listenAddr
	}

	outs := make(paxos.ChanPutCloserTo)

	self := util.RandHexString(idBits)
	st := store.New()
	seqn := uint64(0)
	if *attachAddr == "" { // we are the only node in a new cluster
		seqn = addPublicAddr(st, seqn + 1, self, *publishAddr)
		seqn = addMember(st, seqn + 1, self, *listenAddr)
		seqn = claimSlot(st, seqn + 1, "1", self)
		seqn = claimLeader(st, seqn + 1, self)
		seqn = claimSlot(st, seqn + 1, "2", "")
		seqn = claimSlot(st, seqn + 1, "3", "")
		seqn = claimSlot(st, seqn + 1, "4", "")
		seqn = claimSlot(st, seqn + 1, "5", "")
	} else {
		c, err := client.Dial(*attachAddr)
		if err != nil {
			panic(err)
		}

		path := prefix + "/junta/info/"+ self +"/public-addr"
		_, err = c.Set(path, *publishAddr, store.Clobber)
		if err != nil {
			panic(err)
		}

		var snap string
		seqn, snap, err = c.Join(self, *listenAddr)
		if err != nil {
			panic(err)
		}

		ch := make(chan store.Event)
		st.Wait(seqn + alpha, ch)
		st.Apply(1, snap)

		go func() {
			<-ch
			activate(st, self, prefix, c)
		}()

		// TODO sink needs a way to pick up missing values if there are any
		// gaps in its sequence
	}
	mg := paxos.NewManager(self, seqn, alpha, st, outs)

	if *attachAddr == "" {
		// Skip ahead alpha steps so that the registrar can provide a
		// meaningful cluster.
		for i := seqn + 1; i < seqn + alpha; i++ {
			go st.Apply(i, store.Nop)
		}
	}

	listener, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		panic(err)
	}

	sv := &server.Server{*listenAddr, st, mg, self, prefix}

	go func() {
		panic(mon.Monitor(self, prefix, st))
	}()

	go func() {
		panic(sv.Serve(listener))
	}()

	go func() {
		panic(sv.ListenAndServeUdp(outs))
	}()

	for {
		st.Apply(mg.Recv())
	}
}

func addPublicAddr(st *store.Store, seqn uint64, self, addr string) uint64 {
	// TODO pull out path as a const
	path := "/junta/info/"+ self +"/public-addr"
	mx, err := store.EncodeSet(path, addr, store.Missing)
	if err != nil {
		panic(err)
	}
	st.Apply(seqn, mx)
	return seqn
}

func addMember(st *store.Store, seqn uint64, self, addr string) uint64 {
	// TODO pull out path as a const
	mx, err := store.EncodeSet("/junta/members/"+self, addr, store.Missing)
	if err != nil {
		panic(err)
	}
	st.Apply(seqn, mx)
	return seqn
}

func claimSlot(st *store.Store, seqn uint64, slot, self string) uint64 {
	// TODO pull out path as a const
	mx, err := store.EncodeSet("/junta/slot/"+slot, self, store.Missing)
	if err != nil {
		panic(err)
	}
	st.Apply(seqn, mx)
	return seqn
}

func claimLeader(st *store.Store, seqn uint64, self string) uint64 {
	// TODO pull out path as a const
	mx, err := store.EncodeSet("/junta/leader", self, store.Missing)
	if err != nil {
		panic(err)
	}
	st.Apply(seqn, mx)
	return seqn
}
