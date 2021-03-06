package signer

import (
	"fmt"
	"github.com/miekg/dns"
	"github.com/miekg/pkcs11"
	"math/rand"
	"os"
	"io"
	"sort"
	"strings"
	"time"
)

// SignArgs contains all the args needed to sign a file.
type SignArgs struct {
        Zone        string    // Zone name
        File        io.Reader // File path
        Output      io.Writer // Out path
        SignExpDate time.Time // Expiration date for the signature.
        CreateKeys  bool      // If True, the sign process creates new keys for the signature.
        NSEC3       bool      // If true, the zone is signed using NSEC3
        OptOut      bool      // If true and NSEC3 is true, the zone is signed using OptOut NSEC3 flag.
        MinTTL      uint32 // Min TTL ;-)
        RRs         RRArray     // RRs
}


// ReadAndParseZone parses a DNS zone file and returns an array of RRs and the zone minTTL.
// It also updates the serial in the SOA record if updateSerial is true.
func ReadAndParseZone(args *SignArgs, updateSerial bool) (RRArray, error) {

	rrs := make(RRArray, 0)

        if args.Zone[len(args.Zone)-1] != '.' {
                args.Zone = args.Zone + "."
        }

	zone := dns.NewZoneParser(args.File, "", "")
	if err := zone.Err(); err != nil {
		return nil, err
	}
	for rr, ok := zone.Next(); ok; rr, ok = zone.Next() {
		rrs = append(rrs, rr)
		if rr.Header().Rrtype == dns.TypeSOA {
			var soa *dns.SOA
			soa = rr.(*dns.SOA)
			args.MinTTL = soa.Minttl
			// UPDATING THE SERIAL
			if updateSerial {
				rr.(*dns.SOA).Serial += 2
			}
		}
	}
	sort.Sort(rrs)
	return rrs, nil
}

func AddNSEC13(args *SignArgs)  {
	if args.NSEC3 {
                for {
                        if err := args.RRs.AddNSEC3Records(args.Zone, args.OptOut); err == nil {
                                break
                        }
                }
        } else {
                args.RRs.AddNSECRecords(args.Zone)
        }
}

// CreateNewDNSKEY creates a new DNSKEY RR, using the parameters provided.
func CreateNewDNSKEY(zone string, flags uint16, algorithm uint8, ttl uint32, publicKey string) *dns.DNSKEY {
	return &dns.DNSKEY{
		Flags:     flags,
		Protocol:  3, // RFC4034 2.1.2
		Algorithm: algorithm,
		Hdr: dns.RR_Header{
			Name:   zone,
			Rrtype: dns.TypeDNSKEY,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		},
		PublicKey: publicKey,
	}
}

// CreateNewRRSIG creates a new RRSIG RR, using the parameters provided.
func CreateNewRRSIG(zone string, dnsKeyRR *dns.DNSKEY, expDate time.Time, rrSetTTL uint32) *dns.RRSIG {
	if expDate.IsZero() {
		expDate = time.Now().AddDate(1, 0, 0)
	}
	return &dns.RRSIG{
		Hdr: dns.RR_Header{
			// Uses RRset TTL, not key TTL
			// (RFC4034, 3: The TTL value of an RRSIG RR MUST match the TTL value of the RRset it covers)
			Ttl: rrSetTTL,
		},
		Algorithm:  dnsKeyRR.Algorithm,
		SignerName: strings.ToLower(zone),
		KeyTag:     dnsKeyRR.KeyTag(),
		Inception:  uint32(time.Now().Unix()),
		Expiration: uint32(expDate.Unix()),
	}
}

// generateSalt returns a salt based on a random string seeded on current time.
func generateSalt() string {
	rand.Seed(time.Now().UnixNano())
	r := rand.Int31()
	s := fmt.Sprintf("%x", r)
	return s
}

// removeDuplicates removes the duplicates from an array of object handles.
func removeDuplicates(objs []pkcs11.ObjectHandle) []pkcs11.ObjectHandle {
	encountered := map[pkcs11.ObjectHandle]bool{}
	result := make([]pkcs11.ObjectHandle, 0)
	for _, o := range objs {
		if !encountered[o] {
			encountered[o] = true
			result = append(result, o)
		}
	}
	return result
}

// FilesExist returns an error if any of the paths received as args does not point to a readable file.
func FilesExist(filepaths ...string) error {
	for _, path := range filepaths {
		_, err := os.Stat(path)
		if err != nil || os.IsNotExist(err) {
			return fmt.Errorf("File %s doesn't exist or it has not reading permissions\n", path)
		}
	}
	return nil
}
