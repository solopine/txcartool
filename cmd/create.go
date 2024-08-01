package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/uuid"
	"io"
	"os"
	"path"
	"strings"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-unixfsnode/data/builder"
	"github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"
	dagpb "github.com/ipld/go-codec-dagpb"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/urfave/cli/v2"
)

// CreateCar creates a car
func CreateCar(c *cli.Context) error {
	var err error

	if !c.IsSet("dest") {
		return fmt.Errorf("a file destination must be specified")
	}
	destDir := c.String("dest")

	var key uuid.UUID
	if !c.IsSet("key") {
		key = uuid.New()
	} else {
		keyStr := c.String("key")
		key, err = uuid.Parse(keyStr)
		if err != nil {
			return fmt.Errorf("key is not uuid")
		}
	}

	deskFile := path.Join(destDir, key.String()+".tmp.car")

	// make a cid with the right length that we eventually will patch with the root.
	hasher, err := multihash.GetHasher(multihash.SHA2_256)
	if err != nil {
		return err
	}
	digest := hasher.Sum([]byte{})
	hash, err := multihash.Encode(digest, multihash.SHA2_256)
	if err != nil {
		return err
	}
	proxyRoot := cid.NewCidV1(uint64(multicodec.DagPb), hash)

	options := []car.Option{}
	options = []car.Option{blockstore.WriteAsCarV1(true)}

	cdest, err := blockstore.OpenReadWrite(deskFile, []cid.Cid{proxyRoot}, options...)
	if err != nil {
		return err
	}

	// Write the unixfs blocks into the store.
	root, err := writeFilesWithMem(c.Context, false, cdest, key)
	if err != nil {
		return err
	}

	if err := cdest.Finalize(); err != nil {
		return err
	}

	// re-open/finalize with the final root.
	err = car.ReplaceRootsInFile(deskFile, []cid.Cid{root})
	if err != nil {
		return err
	}

	pieceCid, pieceSize, carSize, err := genCommp(deskFile)
	if err != nil {
		return err
	}
	fmt.Printf("%s\t%s\t%d\t%d\n", key.String(), pieceCid.String(), pieceSize, carSize)

	newDeskFile := path.Join(destDir, pieceCid.String()+".car")
	os.Rename(deskFile, newDeskFile)

	return nil
}

func writeFilesWithMem(ctx context.Context, noWrap bool, bs *blockstore.ReadWrite, key uuid.UUID) (cid.Cid, error) {

	ls := cidlink.DefaultLinkSystem()
	ls.TrustedStorage = true
	ls.StorageReadOpener = func(_ ipld.LinkContext, l ipld.Link) (io.Reader, error) {
		cl, ok := l.(cidlink.Link)
		if !ok {
			return nil, fmt.Errorf("not a cidlink")
		}
		blk, err := bs.Get(ctx, cl.Cid)
		if err != nil {
			return nil, err
		}
		return bytes.NewBuffer(blk.RawData()), nil
	}
	ls.StorageWriteOpener = func(_ ipld.LinkContext) (io.Writer, ipld.BlockWriteCommitter, error) {
		buf := bytes.NewBuffer(nil)
		return buf, func(l ipld.Link) error {
			cl, ok := l.(cidlink.Link)
			if !ok {
				return fmt.Errorf("not a cidlink")
			}
			blk, err := blocks.NewBlockWithCid(buf.Bytes(), cl.Cid)
			if err != nil {
				return err
			}
			bs.Put(ctx, blk)
			return nil
		}, nil
	}

	topLevel := make([]dagpb.PBLink, 0, 1)
	{
		r, name, err := createReader1(key)
		if err != nil {
			return cid.Undef, err
		}

		l, size, err := buildUnixFS(r, &ls)
		if err != nil {
			return cid.Undef, err
		}
		if noWrap {
			rcl, ok := l.(cidlink.Link)
			if !ok {
				return cid.Undef, fmt.Errorf("could not interpret %s", l)
			}
			return rcl.Cid, nil
		}

		entry, err := builder.BuildUnixFSDirectoryEntry(name, int64(size), l)
		if err != nil {
			return cid.Undef, err
		}
		topLevel = append(topLevel, entry)
	}
	{
		r, name, err := createReader2(key)
		if err != nil {
			return cid.Undef, err
		}

		l, size, err := buildUnixFS(r, &ls)
		if err != nil {
			return cid.Undef, err
		}
		if noWrap {
			rcl, ok := l.(cidlink.Link)
			if !ok {
				return cid.Undef, fmt.Errorf("could not interpret %s", l)
			}
			return rcl.Cid, nil
		}

		entry, err := builder.BuildUnixFSDirectoryEntry(name, int64(size), l)
		if err != nil {
			return cid.Undef, err
		}
		topLevel = append(topLevel, entry)
	}

	// make a directory for the file(s).

	root, _, err := builder.BuildUnixFSDirectory(topLevel, &ls)
	if err != nil {
		return cid.Undef, nil
	}
	rcl, ok := root.(cidlink.Link)
	if !ok {
		return cid.Undef, fmt.Errorf("could not interpret %s", root)
	}

	return rcl.Cid, nil
}

func buildUnixFS(r io.Reader, ls *ipld.LinkSystem) (ipld.Link, uint64, error) {
	outLnk, sz, err := builder.BuildUnixFSFile(r, "", ls)
	if err != nil {
		return nil, 0, err
	}
	return outLnk, sz, nil
}

type memReader struct {
	data string
}

func newMemReader(data string) io.Reader {
	mr := memReader{
		data,
	}
	return strings.NewReader(mr.data)
}

func createReader1(key uuid.UUID) (io.Reader, string, error) {
	name := "readme.txt"
	data := key.String()
	return strings.NewReader(data), name, nil
}

func createReader2(key uuid.UUID) (io.Reader, string, error) {
	name := key.String() + ".dat"

	r := io.MultiReader(strings.NewReader(key.String()), NewZoReader(1<<34), strings.NewReader(key.String()))

	return r, name, nil
}
