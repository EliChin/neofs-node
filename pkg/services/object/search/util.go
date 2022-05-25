package searchsvc

import (
	"sync"

	"github.com/nspcc-dev/neofs-node/pkg/core/client"
	"github.com/nspcc-dev/neofs-node/pkg/core/netmap"
	"github.com/nspcc-dev/neofs-node/pkg/local_object_storage/engine"
	internalclient "github.com/nspcc-dev/neofs-node/pkg/services/object/internal/client"
	"github.com/nspcc-dev/neofs-node/pkg/services/object/util"
	"github.com/nspcc-dev/neofs-node/pkg/services/object_manager/placement"
	cid "github.com/nspcc-dev/neofs-sdk-go/container/id"
	addressSDK "github.com/nspcc-dev/neofs-sdk-go/object/address"
	oidSDK "github.com/nspcc-dev/neofs-sdk-go/object/id"
)

type uniqueIDWriter struct {
	mtx sync.Mutex

	written map[string]struct{}

	writer IDListWriter
}

type clientConstructorWrapper struct {
	constructor ClientConstructor
}

type clientWrapper struct {
	client client.MultiAddressClient
}

type storageEngineWrapper engine.StorageEngine

type traverseGeneratorWrapper util.TraverserGenerator

type nmSrcWrapper struct {
	nmSrc netmap.Source
}

func newUniqueAddressWriter(w IDListWriter) IDListWriter {
	return &uniqueIDWriter{
		written: make(map[string]struct{}),
		writer:  w,
	}
}

func (w *uniqueIDWriter) WriteIDs(list []oidSDK.ID) error {
	w.mtx.Lock()

	for i := 0; i < len(list); i++ { // don't use range, slice mutates in body
		s := list[i].String()
		// standard stringer is quite costly, it is better
		// to facilitate the calculation of the key

		if _, ok := w.written[s]; !ok {
			// mark address as processed
			w.written[s] = struct{}{}
			continue
		}

		// exclude processed address
		list = append(list[:i], list[i+1:]...)
		i--
	}

	w.mtx.Unlock()

	return w.writer.WriteIDs(list)
}

func (c *clientConstructorWrapper) get(info client.NodeInfo) (searchClient, error) {
	clt, err := c.constructor.Get(info)
	if err != nil {
		return nil, err
	}

	return &clientWrapper{
		client: clt,
	}, nil
}

func (c *clientWrapper) searchObjects(exec *execCtx, info client.NodeInfo) ([]oidSDK.ID, error) {
	if exec.prm.forwarder != nil {
		return exec.prm.forwarder(info, c.client)
	}

	var sessionInfo *util.SessionInfo

	if tok := exec.prm.common.SessionToken(); tok != nil {
		sessionInfo = &util.SessionInfo{
			ID:    tok.ID(),
			Owner: tok.Issuer(),
		}
	}

	key, err := exec.svc.keyStore.GetKey(sessionInfo)
	if err != nil {
		return nil, err
	}

	var prm internalclient.SearchObjectsPrm

	prm.SetContext(exec.context())
	prm.SetClient(c.client)
	prm.SetPrivateKey(key)
	prm.SetSessionToken(exec.prm.common.SessionToken())
	prm.SetBearerToken(exec.prm.common.BearerToken())
	prm.SetTTL(exec.prm.common.TTL())
	prm.SetXHeaders(exec.prm.common.XHeaders())
	prm.SetNetmapEpoch(exec.curProcEpoch)
	prm.SetContainerID(exec.containerID())
	prm.SetFilters(exec.searchFilters())

	res, err := internalclient.SearchObjects(prm)
	if err != nil {
		return nil, err
	}

	return res.IDList(), nil
}

func (e *storageEngineWrapper) search(exec *execCtx) ([]oidSDK.ID, error) {
	r, err := (*engine.StorageEngine)(e).Select(new(engine.SelectPrm).
		WithFilters(exec.searchFilters()).
		WithContainerID(exec.containerID()),
	)
	if err != nil {
		return nil, err
	}

	return idsFromAddresses(r.AddressList()), nil
}

func idsFromAddresses(addrs []*addressSDK.Address) []oidSDK.ID {
	ids := make([]oidSDK.ID, len(addrs))

	for i := range addrs {
		ids[i], _ = addrs[i].ObjectID()
	}

	return ids
}

func (e *traverseGeneratorWrapper) generateTraverser(cnr *cid.ID, epoch uint64) (*placement.Traverser, error) {
	a := addressSDK.NewAddress()
	a.SetContainerID(*cnr)

	return (*util.TraverserGenerator)(e).GenerateTraverser(a, epoch)
}

func (n *nmSrcWrapper) currentEpoch() (uint64, error) {
	return n.nmSrc.Epoch()
}
