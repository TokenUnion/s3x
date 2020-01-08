package s3x

import (
	"context"
	"sync"

	pb "github.com/RTradeLtd/TxPB/v3/go"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
)

/* Design Notes
---------------

Internal functions should never claim or release locks.
Any claiming or releasing of locks should be done in the public setter+getter functions.
The reason for this is so that we can enable easy reuse of internal code.
*/

var (
	dsKey       = datastore.NewKey("ledgerstatekey")
	dsPrefix    = datastore.NewKey("ledgerRoot")
	dsBucketKey = datastore.NewKey("b")
)

// ledgerStore is an internal bookkeeper that
// maps buckets to ipfs cids and keeps a local cache of object names to hashes
//
// Bucket root hashes are saved in the provided data store.
// Object hashes are saved in ipfs and cached in memory,
// Object data is saved in ipfs.
type ledgerStore struct {
	sync.RWMutex //to be changed to per bucket name, once datastore saves each bucket separatory
	ds           datastore.Batching
	dag          pb.NodeAPIClient //to be used as direct access to ipfs to optimise algorithm
	l            *Ledger          //a cache of the values in datastore and ipfs
}

func newLedgerStore(ds datastore.Batching, dag pb.NodeAPIClient) (*ledgerStore, error) {
	ls := &ledgerStore{
		ds:  namespace.Wrap(ds, dsPrefix),
		dag: dag,
		l:   &Ledger{},
	}
	return ls, nil
}

func (ls *ledgerStore) object(ctx context.Context, bucket, object string) (*Object, error) {
	objectHash, err := ls.GetObjectHash(ctx, bucket, object)
	if err != nil {
		return nil, err
	}
	return ipfsObject(ctx, ls.dag, objectHash)
}

func (ls *ledgerStore) objectData(ctx context.Context, bucket, object string) ([]byte, error) {
	obj, err := ls.object(ctx, bucket, object)
	if err != nil {
		return nil, err
	}
	return ipfsBytes(ctx, ls.dag, obj.GetDataHash())
}

// RemoveObject is used to remove a ledger object entry from a ledger bucket entry
func (ls *ledgerStore) RemoveObject(ctx context.Context, bucket, object string) error {
	b, err := ls.getBucket(bucket)
	if err != nil {
		return err
	}
	if b == nil {
		return ErrLedgerBucketDoesNotExist
	}
	err = b.ensureCache(ctx, ls.dag)
	if err != nil {
		return err
	}

	delete(b.Bucket.Objects, object)
	return nil //todo: gc on ipfs
}

// putObject saves an object into the given bucket
func (ls *ledgerStore) putObject(ctx context.Context, bucket, object, objHash string) error {
	b, err := ls.getBucket(bucket)
	if err != nil {
		return err
	}
	if b == nil {
		return ErrLedgerBucketDoesNotExist
	}
	if err := b.ensureCache(ctx, ls.dag); err != nil {
		return err
	}
	if b.Bucket.Objects == nil {
		b.Bucket.Objects = make(map[string]string)
	}
	b.Bucket.Objects[object] = objHash
	return ls.saveBucket(ctx, bucket, b.Bucket)
}
