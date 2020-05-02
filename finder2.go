package main

import (
    "bytes"
    //"compress/zlib"
    "encoding/binary"
    "flag"
    "fmt"
    //"io/ioutil"
    //"io"
    "log"
    "os"
    "github.com/seebs/nbt"
    "strings"
)

func getTagByPath(tag nbt.Tag, path string) nbt.Tag {
    pathList := strings.Split(path, "/")

    t := tag
    var ok bool

    for _, s := range pathList {
        t, ok = nbt.TagElement(t, s)
        if ! ok {
            log.Fatal("Element %s not found", s)
        }
    }

    return(t)
}

type ChunkHeader struct {
    CompressedSize uint32
    CompressionType uint8
}

func findBlockInChunkFile(filename string, blockId int) {
    f, err := os.Open(filename)
    if err != nil {
        log.Printf("%v\n", err)
        return
    }

    defer f.Close()

    // Skip header
    f.Seek(8192, 0)

    header := ChunkHeader{}
    err = binary.Read(f, binary.BigEndian, &header)
    if err != nil {
        log.Printf("Failed to read chunk header: %v\n", err)
        return
    }

    log.Printf("Loading %v compressed bytes\n", header.CompressedSize)

    data := make([]byte, header.CompressedSize)
    n, err := f.Read(data)
    if err != nil {
        log.Printf("Failed to read chunk data: %v\n", err)
        return
    }

    if n != int(header.CompressedSize) {
        log.Printf("Expected to read %v bytes but got %v\n", header.CompressedSize, n)
        return
    }

    buf := bytes.NewBuffer(data)

    tag, _, err := nbt.Load(buf)
    if err != nil {
        log.Printf("Could not read chunk NBT data: %v\n", err)
        return
    }

    nbt.PrintIndented(os.Stdout, tag)
}

func getBlockId(blockName string, path string, tag nbt.Tag) (blockId int, found bool) {
    found = false
    blockId = -1

    idTag := getTagByPath(tag, path)

    nbElements := nbt.TagLength(idTag)
    //fmt.Printf("nbElements: %v\n", nbElements)
    //fmt.Printf("tag: %v\n", idTag)

    for i := 1; i < nbElements; i++ {
        kv, ok := nbt.TagElement(idTag, i)
        if ! ok {
            log.Fatal("Could not load element ", i)
        }

        k, ok := nbt.TagElement(kv, "K")
        if ! ok {
            log.Fatal("Could not K for element ", i)
        }

        v, ok := nbt.TagElement(kv, "V")
        if ! ok {
            log.Fatal("Could not V for element ", i)
        }

        s, ok := nbt.GetString(k)
        if ! ok {
            log.Fatal("Could not get string value for K")
        }

        if string(s) == blockName {
            found = true
            iv, ok := nbt.GetInt(v)
            if ! ok {
                log.Fatal("Could not get int for blockId")
            }
            blockId = int(iv)
            break
        }
    }

    return blockId, found
}

func main() {
    var blockName string

    flag.StringVar(&blockName, "blockName", "biomesoplenty:hive", "Name of the block to find")
    flag.Parse()

    f, err := os.Open("level.dat")
    if err != nil {
        log.Fatal(err)
    }

    rootTag, _, err := nbt.Load(f)
    if err != nil {
        log.Fatal(err)
    }

    blockId, found := getBlockId(blockName, "FML/Registries/minecraft:blocks/ids", rootTag)
    if found {
        fmt.Printf("blockId: %v\n", blockId)
    } else {
        fmt.Printf("Block not found\n")
    }

    f.Close()

    for _, filename := range(flag.Args()) {
        fmt.Printf("Looking in %v\n", filename)

        findBlockInChunkFile(filename, blockId)
    }
}
