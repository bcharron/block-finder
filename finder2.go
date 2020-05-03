package main

import (
    "bytes"
    "compress/zlib"
    "encoding/binary"
    "flag"
    "fmt"
    "io/ioutil"
    //"io"
    "log"
    "os"
    "github.com/seebs/nbt"
    //"reflect"
    "strings"
)

var BlockMap map[uint32]string

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

type ChunkOffset struct {
    Offset [3]uint8
    Sectors uint8
}

type ChunkHeader struct {
    CompressedSize uint32
    CompressionType uint8
}

func loadChunkData(f *os.File, offset int64) (out *bytes.Buffer) {
    header := ChunkHeader{}

    f.Seek(offset, 0)

    err := binary.Read(f, binary.BigEndian, &header)
    if err != nil {
        log.Printf("Failed to read chunk header: %v\n", err)
        return(nil)
    }

    if header.CompressedSize - 1 > 16 * 1024 * 1024 {
        log.Printf("Chunk seems too big, skipping. Size: %v\n", header.CompressedSize - 1)
        return(nil)
    }

    // log.Printf("Loading %v compressed bytes\n", header.CompressedSize - 1)

    data := make([]byte, header.CompressedSize - 1)
    n, err := f.Read(data)
    if err != nil {
        log.Printf("Failed to read chunk data: %v\n", err)
        return(nil)
    }

    if n != int(header.CompressedSize - 1) {
        log.Printf("Expected to read %v bytes but got %v\n", header.CompressedSize - 1, n)
        return(nil)
    }

    buf := bytes.NewBuffer(data)

    zlibReader, err := zlib.NewReader(buf)
    if err != nil {
        log.Fatal(err)
    }

    content, err := ioutil.ReadAll(zlibReader)
    if err != nil {
        log.Fatal(err)
    }

    out = bytes.NewBuffer(content)

    return(out)
}

func findBlockInChunk(blockId uint32, data *bytes.Buffer) {
    tag, _, err := nbt.Load(data)
    if err != nil {
        log.Printf("Could not read chunk NBT data: %v\n", err)
        return
    }

    level := getTagByPath(tag, "Level")
    chunkX, _ := nbt.TagElement(level, "xPos")
    chunkZ, _ := nbt.TagElement(level, "zPos")
    sections := getTagByPath(level, "Sections")
    nbElements := nbt.TagLength(sections)

    for i := 0; i < nbElements; i++ {
        // fmt.Printf("Getting section %v\n", i)
        section, ok := nbt.TagElement(sections, i)
        if ! ok {
            log.Printf("Failed to get element %v", i)
            break
        }

        /*
        element, ok := nbt.TagElement(section, "Y")
        b, ok := nbt.GetByte(element)
        fmt.Printf("Y: %d\n", b)
        */

        blocksLSB, ok := nbt.TagElement(section, "Blocks")
        nbBlocks := nbt.TagLength(blocksLSB)
        if nbBlocks != 4096 {
            fmt.Printf("Blocks array have a weird length: %d\n", nbBlocks)
            continue
        }

        lsb := blocksLSB.(nbt.ByteArray)
        blocks := make([]uint32, len(lsb))
        for x, b := range(lsb) {
            blocks[x] = uint32(uint8(b))
        }

        // add1 := make([]uint32, 4096)
        // add2 := make([]uint32, 4096)

        blocksMSB, ok := nbt.TagElement(section, "Add")
        if ok {
            nbAdds := nbt.TagLength(blocksMSB)
            if nbAdds != 2048 {
                fmt.Printf("Add array have a weird length: %d\n", nbAdds)
                continue
            }

            array := blocksMSB.(nbt.ByteArray)

            for x, b := range(array) {
                blocks[x << 1] |= uint32(uint8(b) & 0x0f) << 8
                blocks[(x << 1) + 1] |= uint32((uint8(b) & 0xf0) >> 4) << 8
            }
        }

        blocksMSB2, ok := nbt.TagElement(section, "Add2")
        if ok {
            nbAdds := nbt.TagLength(blocksMSB2)
            if nbAdds != 2048 {
                fmt.Printf("Add2 array have a weird length: %d\n", nbAdds)
                continue
            }

            array := blocksMSB2.(nbt.ByteArray)

            for x, b := range(array) {
                blocks[x << 1] |= uint32(uint8(b) & 0x0f) << 12
                blocks[(x << 1) + 1] |= uint32((uint8(b) & 0xf0) >> 4) << 12
            }
        }

        for _, b := range(blocks) {
            // block := b + (add1[x] << 8) + (add2[x] << 12)

            if b == blockId {
                fmt.Printf("WINNER! Found blockId %v in chunk at X:%v, Z:%v\n", b, chunkX, chunkZ)
            }
        }
    }

    // Go through tile entities
    tileEntities, ok := nbt.TagElement(level, "TileEntities")
    if !ok {
        return
    }

    nbTileEntities := nbt.TagLength(tileEntities)

    blockName := BlockMap[blockId]

    // fmt.Printf("Tile entities: %v\n", nbTileEntities)
    for i := 0; i < nbTileEntities; i++ {
        tileEntity, ok := nbt.TagElement(tileEntities, i)
        if ! ok {
            log.Printf("Failed to get element %v", i)
            break
        }

        entityNameTag, ok := nbt.TagElement(tileEntity, "id")
        entityName, ok := nbt.GetString(entityNameTag)
        // fmt.Printf("Entity: %v\n", string(entityName))
        if string(entityName) == blockName {
            fmt.Printf("WINNER! Found block as Tile Entity %v in chunk at X:%v, Z:%v\n", entityName, chunkX, chunkZ)
        }
    }
}

func findBlockInChunkFile(filename string, blockId uint32) {
    f, err := os.Open(filename)
    if err != nil {
        log.Printf("%v\n", err)
        return
    }

    defer f.Close()

    offsetsTable := make([]ChunkOffset, 1024)
    err = binary.Read(f, binary.BigEndian, offsetsTable)
    if err != nil {
        log.Printf("Failed to read offset table: %v\n", err)
        return
    }

    // Skip timestamp table
    // f.Seek(4096, 0)

    for _, entry := range(offsetsTable) {
        // fmt.Printf("%v %v %v\n", entry.Offset[0], entry.Offset[1], entry.Offset[2])
        offset := uint32(uint32(entry.Offset[0]) << 16 + uint32(entry.Offset[1]) << 8 + uint32(entry.Offset[2])) * 4096
        // fmt.Printf("Chunk offset: %v\n", offset)
        if offset >= 8192 && offset < 1024*1024*16 {
            data := loadChunkData(f, int64(offset))
            if data != nil {
                findBlockInChunk(uint32(blockId), data)
            } else {
                fmt.Printf("Could not load chunk at offset %v\n", offset)
            }
        } else {
            // fmt.Printf("Skipping chunk %v because its offset is %v\n", i, offset)
        }
    }

    // nbt.PrintIndented(os.Stdout, sections)
}

func getBlocks(levelTag nbt.Tag) (blockMap map[uint32]string) {
    blockMap = make(map[uint32]string)
    //found = false
    //blockId = math.MaxUint32

    idTag := getTagByPath(levelTag, "FML/Registries/minecraft:blocks/ids")

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

        iv, ok := nbt.GetInt(v)
        if ! ok {
            log.Fatal("Could not get int for blockId")
        }

        blockMap[uint32(iv)] = string(s)
        /*
        if string(s) == blockName {
            found = true
            blockId = uint32(iv)
            break
        }
        */
    }

    return(blockMap)
}

func main() {
    var blockName string
    var listBlocks bool

    flag.StringVar(&blockName, "blockName", "please set block name", "Name of the block to find")
    flag.BoolVar(&listBlocks, "listBlocks", false, "Show the list of block and their IDs from level.dat")
    flag.Parse()

    f, err := os.Open("level.dat")
    if err != nil {
        log.Fatal(err)
    }

    rootTag, _, err := nbt.Load(f)
    if err != nil {
        log.Fatal(err)
    }

    if listBlocks {
        blockName = "xxxxxxxxxxxxxxxxxxxxxx"
    }

    BlockMap = getBlocks(rootTag)

    found := false

    var blockId uint32

    if listBlocks {
        for k, v := range(BlockMap) {
            fmt.Printf("%v %v\n", k, v)
        }
        return
    } else {
        for id, name := range(BlockMap) {
            if name == blockName {
                found = true
                blockId = id
                break
            }
        }
    }

    if found {
        fmt.Printf("blockId: %v\n", blockId)
    } else {
        fmt.Printf("No Block ID found for block name\"%s\"\n", blockName)
        return
    }

    f.Close()

    for _, filename := range(flag.Args()) {
        fmt.Printf("Looking in %v\n", filename)

        findBlockInChunkFile(filename, blockId)
    }
}
