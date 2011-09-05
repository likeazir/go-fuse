package fuse
import (
	"os"
	"sync"
	"unsafe"
)

// openedFile stores either an open dir or an open file.
type openedFile struct {
	Handled

	// O_CREAT, O_TRUNC, etc.
	OpenFlags uint32

	// FOPEN_KEEP_CACHE and friends.
	FuseFlags uint32

	dir  rawDir
	file File
}

type fileSystemMount struct {
	// The file system we mounted here.
	fs FileSystem

	// Node that we were mounted on.
	mountInode *inode

	// Options for the mount.
	options *FileSystemOptions

	// Protects Children hashmaps within the mount.  treeLock
	// should be acquired before openFilesLock
	treeLock sync.RWMutex

	// Manage filehandles of open files.
	openFiles HandleMap
}



func (me *fileSystemMount) setOwner(attr *Attr) {
	if me.options.Owner != nil {
		attr.Owner = *me.options.Owner
	}
}
func (me *fileSystemMount) fileInfoToEntry(fi *os.FileInfo, out *EntryOut) {
	SplitNs(me.options.EntryTimeout, &out.EntryValid, &out.EntryValidNsec)
	SplitNs(me.options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)
	if !fi.IsDirectory() {
		fi.Nlink = 1
	}

	CopyFileInfo(fi, &out.Attr)
	me.setOwner(&out.Attr)
}

	
func (me *fileSystemMount) fileInfoToAttr(fi *os.FileInfo, out *AttrOut) {
	CopyFileInfo(fi, &out.Attr)
	SplitNs(me.options.AttrTimeout, &out.AttrValid, &out.AttrValidNsec)
	me.setOwner(&out.Attr)
}

func (me *FileSystemConnector) getOpenedFile(h uint64) *openedFile {
	b := (*openedFile)(unsafe.Pointer(DecodeHandle(h)))
	return b
}

func (me *fileSystemMount) unregisterFileHandle(handle uint64, node *inode) *openedFile {
	obj := me.openFiles.Forget(handle)
	opened := (*openedFile)(unsafe.Pointer(obj))
	node.OpenFilesMutex.Lock()
	defer node.OpenFilesMutex.Unlock()

	idx := -1
	for i, v := range node.OpenFiles {
		if v == opened {
			idx = i
			break
		}
	}

	l := len(node.OpenFiles)
	node.OpenFiles[idx] = node.OpenFiles[l-1]
	node.OpenFiles = node.OpenFiles[:l-1]

	return opened
}

func (me *fileSystemMount) registerFileHandle(node *inode, dir rawDir, f File, flags uint32) (uint64, *openedFile) {
	node.OpenFilesMutex.Lock()
	defer node.OpenFilesMutex.Unlock()
	b := &openedFile{
		dir:             dir,
		file:            f,
		OpenFlags:       flags,
	}

	withFlags, ok := f.(*WithFlags)
	if ok {
		b.FuseFlags = withFlags.Flags
		f = withFlags.File
	}

	node.OpenFiles = append(node.OpenFiles, b)
	handle := me.openFiles.Register(&b.Handled)
	return handle, b
}