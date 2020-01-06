package byzq

import (
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
)

var priv *ecdsa.PrivateKey

var pemKeyData = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIANyDBAupB6O86ORJ1u95Cz6C+lz3x2WKOFntJNIesvioAoGCCqGSM49
AwEHoUQDQgAE+pBXRIe0CI3vcdJwSvU37RoTqlPqEve3fcC36f0pY/X9c9CsgkFK
/sHuBztq9TlUfC0REC81NRqRgs6DTYJ/4Q==
-----END EC PRIVATE KEY-----`

func TestMain(m *testing.M) {
	silentLogger := log.New(ioutil.Discard, "", log.LstdFlags)
	grpclog.SetLogger(silentLogger)
	grpc.EnableTracing = false
	var err error
	priv, err = ParseKey(pemKeyData)
	if err != nil {
		log.Fatalln("couldn't parse private key")
	}
	res := m.Run()
	os.Exit(res)
}

var authQTests = []struct {
	n   int
	f   int // expected value
	q   int // expected value
	err string
}{
	{-1, 0, 1, "Byzantine quorum require n>3f replicas; only got n=-1, yielding f=0"},
	{0, 0, 1, "Byzantine quorum require n>3f replicas; only got n=0, yielding f=0"},
	{1, 0, 1, "Byzantine quorum require n>3f replicas; only got n=1, yielding f=0"},
	{2, 0, 2, "Byzantine quorum require n>3f replicas; only got n=2, yielding f=0"},
	{3, 0, 2, "Byzantine quorum require n>3f replicas; only got n=3, yielding f=0"},
	{4, 1, 2, ""},
	{5, 1, 3, ""},
	{6, 1, 3, ""},
	{7, 2, 4, ""},
	{8, 2, 5, ""},
	{9, 2, 5, ""},
	{10, 3, 6, ""},
	{11, 3, 7, ""},
	{12, 3, 7, ""},
	{13, 4, 8, ""},
	{14, 4, 9, ""},
}

func TestNewAuthDataQ(t *testing.T) {
	for _, test := range authQTests {
		bq, err := NewAuthDataQ(test.n, priv, &priv.PublicKey)
		if err != nil {
			if err.Error() != test.err {
				t.Errorf("got '%v', expected '%v'", err.Error(), test.err)
			}
			continue
		}
		if bq.f != test.f {
			t.Errorf("got f=%d, expected f=%d", bq.f, test.f)
		}
		if bq.q != test.q {
			t.Errorf("got q=%d, expected q=%d", bq.q, test.q)
		}
	}
}

var (
	myVal  = &Value{C: &Content{Key: "Winnie", Value: "Poo", Timestamp: 1}}
	myVal2 = &Value{C: &Content{Key: "Winnie", Value: "Poop", Timestamp: 2}}
	myVal3 = &Value{C: &Content{Key: "Winnie", Value: "Pooop", Timestamp: 3}}
	myVal4 = &Value{C: &Content{Key: "Winnie", Value: "Poooop", Timestamp: 4}}
)

var authReadQFTests = []struct {
	name     string
	replies  []*Value
	expected *Content
	rq       bool
}{
	{
		"nil input",
		nil,
		nil,
		false,
	},
	{
		"len=0 input",
		[]*Value{},
		nil,
		false,
	},
	{
		"no quorum (I)",
		[]*Value{
			myVal,
		},
		nil,
		false,
	},
	{
		"no quorum (II)",
		[]*Value{
			myVal,
			myVal,
		},
		nil,
		false,
	},
	{
		"quorum (I)",
		[]*Value{
			myVal,
			myVal,
			myVal,
		},
		myVal.C,
		true,
	},
	{
		"quorum (II)",
		[]*Value{
			myVal2,
			myVal,
			myVal,
		},
		myVal2.C,
		true,
	},
	{
		"quorum (III)",
		[]*Value{
			myVal2,
			myVal,
			myVal3,
		},
		myVal3.C,
		true,
	},
	{
		"quorum (IV)",
		[]*Value{
			myVal2,
			myVal,
			myVal3,
			myVal4,
		},
		myVal4.C,
		true,
	},
	{
		"quorum (V)",
		[]*Value{
			myVal,
			myVal,
			myVal,
			myVal,
		},
		myVal.C,
		true,
	},
	{
		"bast-case quorum",
		[]*Value{
			myVal,
			myVal,
			myVal,
		},
		myVal.C,
		true,
	},
	{
		"worst-case quorum",
		[]*Value{
			myVal,
			myVal,
			myVal,
			myVal,
		},
		myVal.C,
		true,
	},
}

func TestAuthDataQ(t *testing.T) {
	qspec, err := NewAuthDataQ(4, priv, &priv.PublicKey)
	if err != nil {
		t.Error(err)
	}
	for _, test := range authReadQFTests {
		for i, r := range test.replies {
			test.replies[i], err = qspec.Sign(r.C)
			if err != nil {
				t.Fatal("Failed to sign message")
			}
		}

		qfuncs := []struct {
			name string
			qf   func([]*Value) (*Content, bool)
		}{
			{"(NoSignVerification)ReadQF(4,1)", qspec.ReadQF},
			{"SequentialVerifyReadQFReadQF(4,1)", qspec.SequentialVerifyReadQF},
			{"ConcurrentVerifyIndexChanReadQF(4,1)", qspec.ConcurrentVerifyIndexChanReadQF},
			{"VerfiyLastReplyFirstReadQF(4,1)", qspec.VerfiyLastReplyFirstReadQF},
			{"ConcurrentVerifyWGReadQF(4,1)", qspec.ConcurrentVerifyWGReadQF},
		}

		for _, qfunc := range qfuncs {
			t.Run(fmt.Sprintf("%s %s", qfunc.name, test.name), func(t *testing.T) {
				reply, byzquorum := qfunc.qf(test.replies)
				if byzquorum != test.rq {
					t.Errorf("got %t, want %t", byzquorum, test.rq)
				}
				if reply != nil {
					if !reply.Equal(test.expected) {
						t.Errorf("got %v, want %v as quorum reply", reply, test.expected)
					}
				} else {
					if test.expected != nil {
						t.Errorf("got %v, want %v as quorum reply", reply, test.expected)
					}
				}
			})
		}
	}
}

func BenchmarkAuthDataQ(b *testing.B) {
	qspec, err := NewAuthDataQ(4, priv, &priv.PublicKey)
	if err != nil {
		b.Error(err)
	}
	for _, test := range authReadQFTests {
		if !strings.Contains(test.name, "case") {
			continue
		}
		for i, r := range test.replies {
			test.replies[i], err = qspec.Sign(r.C)
			if err != nil {
				b.Fatal("Failed to sign message")
			}
		}

		qfuncs := []struct {
			name string
			qf   func([]*Value) (*Content, bool)
		}{
			{"(NoSignVerification)ReadQF(4,1)", qspec.ReadQF},
			{"SequentialVerifyReadQFReadQF(4,1)", qspec.SequentialVerifyReadQF},
			{"ConcurrentVerifyIndexChanReadQF(4,1)", qspec.ConcurrentVerifyIndexChanReadQF},
			{"VerfiyLastReplyFirstReadQF(4,1)", qspec.VerfiyLastReplyFirstReadQF},
			{"ConcurrentVerifyWGReadQF(4,1)", qspec.ConcurrentVerifyWGReadQF},
		}

		for _, qfunc := range qfuncs {
			b.Run(fmt.Sprintf("%s %s", qfunc.name, test.name), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					qfunc.qf(test.replies)
				}
			})
		}
	}
}

var authWriteQFTests = []struct {
	name     string
	replies  []*WriteResponse
	expected *WriteResponse
	rq       bool
}{
	{
		"nil input",
		nil,
		nil,
		false,
	},
	{
		"len=0 input",
		[]*WriteResponse{},
		nil,
		false,
	},
	{
		"no quorum (I)",
		[]*WriteResponse{
			{Timestamp: 1},
		},
		nil,
		false,
	},
	{
		"no quorum (II)",
		[]*WriteResponse{
			{Timestamp: 1},
			{Timestamp: 1},
		},
		nil,
		false,
	},
	{
		"no quorum (III)",
		[]*WriteResponse{
			{Timestamp: 1},
			{Timestamp: 2},
			{Timestamp: 3},
			{Timestamp: 4},
		},
		nil,
		false,
	},
	{
		"no quorum (IV)",
		[]*WriteResponse{
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 2},
			{Timestamp: 2},
		},
		nil,
		false,
	},
	{
		"quorum (I)",
		[]*WriteResponse{
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 1},
		},
		&WriteResponse{Timestamp: 1},
		true,
	},
	{
		"quorum (II)",
		[]*WriteResponse{
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 1},
		},
		&WriteResponse{Timestamp: 1},
		true,
	},
	{
		"quorum (III)",
		[]*WriteResponse{
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 2},
		},
		&WriteResponse{Timestamp: 1},
		true,
	},
	{
		"quorum (IV)",
		[]*WriteResponse{
			{Timestamp: 2},
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 1},
		},
		&WriteResponse{Timestamp: 1},
		true,
	},
	{
		"best-case quorum",
		[]*WriteResponse{
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 1},
		},
		&WriteResponse{Timestamp: 1},
		true,
	},
	{
		"worst-case quorum",
		[]*WriteResponse{
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 1},
		},
		&WriteResponse{Timestamp: 1},
		true,
	},
}

func TestAuthDataQW(t *testing.T) {
	qspec, err := NewAuthDataQ(4, priv, &priv.PublicKey)
	if err != nil {
		t.Error(err)
	}
	for _, test := range authWriteQFTests {
		t.Run(fmt.Sprintf("WriteQF(4,1) %s", test.name), func(t *testing.T) {
			req := &Value{C: &Content{Timestamp: 0}}
			if test.expected != nil {
				req = &Value{C: &Content{Timestamp: test.expected.Timestamp}}
			}
			reply, byzquorum := qspec.WriteQF(req, test.replies)
			if byzquorum != test.rq {
				t.Errorf("got %t, want %t", byzquorum, test.rq)
			}
			if !reply.Equal(test.expected) {
				t.Errorf("got %v, want %v as quorum reply", reply, test.expected)
			}
		})
	}
}

func BenchmarkAuthDataQW(b *testing.B) {
	qspec, err := NewAuthDataQ(4, priv, &priv.PublicKey)
	if err != nil {
		b.Error(err)
	}
	for _, test := range authWriteQFTests {
		req := &Value{C: &Content{Timestamp: 0}}
		if test.expected != nil {
			req = &Value{C: &Content{Timestamp: test.expected.Timestamp}}
		}
		if !strings.Contains(test.name, "case") {
			continue
		}
		b.Run(fmt.Sprintf("WriteQF(4,1) %s", test.name), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				qspec.WriteQF(req, test.replies)
			}
		})
	}
}
