package blobovnicza

import (
	apistatus "github.com/nspcc-dev/neofs-sdk-go/client/status"
	addressSDK "github.com/nspcc-dev/neofs-sdk-go/object/address"
	"go.etcd.io/bbolt"
	"go.uber.org/zap"
)

// GetPrm groups the parameters of Get operation.
type GetPrm struct {
	addr *addressSDK.Address
}

// GetRes groups resulting values of Get operation.
type GetRes struct {
	obj []byte
}

// SetAddress sets address of the requested object.
func (p *GetPrm) SetAddress(addr *addressSDK.Address) {
	p.addr = addr
}

// Object returns binary representation of the requested object.
func (p *GetRes) Object() []byte {
	return p.obj
}

// Get reads the object from Blobovnicza by address.
//
// Returns any error encountered that
// did not allow to completely read the object.
//
// Returns apistatus.ObjectNotFound if requested object is not
// presented in Blobovnicza.
func (b *Blobovnicza) Get(prm *GetPrm) (*GetRes, error) {
	var (
		data    []byte
		addrKey = addressKey(prm.addr)
	)

	if err := b.boltDB.View(func(tx *bbolt.Tx) error {
		return b.iterateBuckets(tx, func(lower, upper uint64, buck *bbolt.Bucket) (bool, error) {
			data = buck.Get(addrKey)

			stop := data != nil

			if stop {
				b.log.Debug("object is found in bucket",
					zap.String("binary size", stringifyByteSize(uint64(len(data)))),
					zap.String("range", stringifyBounds(lower, upper)),
				)
			}

			return stop, nil
		})
	}); err != nil {
		return nil, err
	}

	if data == nil {
		var errNotFound apistatus.ObjectNotFound

		return nil, errNotFound
	}

	return &GetRes{
		obj: data,
	}, nil
}
