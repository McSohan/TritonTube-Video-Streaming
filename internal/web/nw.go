// Lab 8: Implement a network video content service (client using consistent hashing)

package web

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"path"
	"sort"
	"strings"
	"sync"
	pb "tritontube/internal/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// NetworkVideoContentService implements VideoContentService using a network of nodes.

// VideoContentAdminService must also be run here
type NetworkVideoContentService struct {
	pb.UnimplementedVideoContentAdminServiceServer
	aliveNodes []node // always sorted by hash
	myAddr     string
	mu         sync.RWMutex
	// grpcServer *grpc.Server
	// listener   net.Listener
}

type node struct {
	addr   string
	hash   uint64
	client pb.StorageServiceClient
	conn   *grpc.ClientConn
}

func NewNetworkVideoContentService(options string) (*NetworkVideoContentService, error) {
	var service *NetworkVideoContentService
	optionStrings := strings.Split(options, ",")
	if len(optionStrings) > 0 {
		service = &NetworkVideoContentService{
			aliveNodes: make([]node, 0), // init with 0 alive nodes
			myAddr:     optionStrings[0],
		}
		if len(optionStrings) > 1 {
			// add the storage servers -- bootstrap
			for _, option := range optionStrings[1:] {
				// fmt.Printf("Adding node %d : %s", id, option)
				err := service.bootstrap(option)
				if err != nil {
					fmt.Printf("Unable to add storage %s", option)
					return nil, err
				}
			}
		}
		// run a grpc server here
		grpcServer := grpc.NewServer()
		pb.RegisterVideoContentAdminServiceServer(grpcServer, service)
		lis, err := net.Listen("tcp", optionStrings[0])
		if err != nil {
			fmt.Printf("Failed to listen: %v", err)
			return nil, err
		}
		// service.grpcServer = grpcServer
		// service.listener = lis
		go func() {

			log.Printf("gRPC server listening on %s", optionStrings[0])
			if err := grpcServer.Serve(lis); err != nil {
				fmt.Printf("Failed to serve: %v", err)
				// TODO: maybe return nil here?
			}
		}()

		// and return an object for NetworkVideoContentService
		return service, nil

	} else {
		return nil, errors.New("invalid options")
	}
}

func (nws *NetworkVideoContentService) Read(videoId string, filename string) ([]byte, error) {

	ctx := context.Background()

	videoHash := hashStringToUint64(path.Join(videoId, filename))
	node, err := nws.getNodeForHash(videoHash)
	if err != nil {
		return nil, err
	}
	response, err := node.client.Read(ctx, &pb.ReadRequest{
		VideoId:  videoId,
		FileName: filename,
	})
	if err != nil {
		fmt.Printf("Read RPC failed: %v", err)
		return nil, err
	}
	return response.FileData, nil
}

func (nws *NetworkVideoContentService) Write(videoId string, filename string, data []byte) error {
	ctx := context.Background()

	videoHash := hashStringToUint64(path.Join(videoId, filename))
	node, err := nws.getNodeForHash(videoHash)
	if err != nil {
		return err
	}

	_, err = node.client.Write(ctx, &pb.WriteRequest{
		VideoId:  videoId,
		FileName: filename,
		FileData: data,
	})
	if err != nil {
		fmt.Printf("Write RPC failed: %v", err)
		return err
	}
	return nil
}

func (nws *NetworkVideoContentService) ListNodes(ctx context.Context, rr *pb.ListNodesRequest) (*pb.ListNodesResponse, error) {
	//assumes a sorted list
	nws.mu.RLock()
	defer nws.mu.RUnlock()
	respList := make([]string, 0)
	for _, node := range nws.aliveNodes {
		respList = append(respList, node.addr)
	}
	return &pb.ListNodesResponse{
		Nodes: respList,
	}, nil
}

func (nws *NetworkVideoContentService) bootstrap(addrs string) error {
	newNodeHash := hashStringToUint64(addrs)
	// create a new node
	conn, err := grpc.NewClient(addrs, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("Failed to connect to server: %v", err)
		return err
	}
	newClient := pb.NewStorageServiceClient(conn)
	newNode := node{
		addr:   addrs,
		hash:   newNodeHash,
		client: newClient,
		conn:   conn,
	}
	nws.mu.Lock()
	defer nws.mu.Unlock()
	nws.aliveNodes = append(nws.aliveNodes, newNode)
	sort.Slice(nws.aliveNodes, func(i, j int) bool {
		return nws.aliveNodes[i].hash < nws.aliveNodes[j].hash
	})
	return nil
}

func (nws *NetworkVideoContentService) AddNode(ctx context.Context, rr *pb.AddNodeRequest) (*pb.AddNodeResponse, error) {

	// fmt.Printf("Adding node %s", rr.NodeAddress)
	migratedFileCount := 0
	q_ctx := context.Background() // should a new context be created?
	newNodeHash := hashStringToUint64(rr.NodeAddress)
	// create a new node
	conn, err := grpc.NewClient(rr.NodeAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("Failed to connect to server: %v", err)
		return nil, err
	}
	newClient := pb.NewStorageServiceClient(conn)
	newNode := node{
		addr:   rr.NodeAddress,
		hash:   newNodeHash,
		client: newClient,
		conn:   conn,
	}
	// get the node's location in the ring
	successorNode, err := nws.getNodeForHash(newNodeHash)
	if err == nil {
		// list all the data from that node if node is found
		data, err := successorNode.client.List(q_ctx, &pb.ListRequest{})
		if err != nil {
			conn.Close()
			return nil, err
		}
		dataToBeMoved := getDataBelongingToNode(data.Files, newNodeHash, successorNode.hash)
		migratedFileCount = len(dataToBeMoved)
		// fmt.Printf("add node: Moving data between %s -> %s\n", successorNode.addr, rr.NodeAddress)
		moveDataBetweenNodes(successorNode.client, newClient, dataToBeMoved)
	}
	nws.mu.Lock()
	defer nws.mu.Unlock()
	//add the node and sort the list
	nws.aliveNodes = append(nws.aliveNodes, newNode)
	sort.Slice(nws.aliveNodes, func(i, j int) bool {
		return nws.aliveNodes[i].hash < nws.aliveNodes[j].hash
	})
	return &pb.AddNodeResponse{MigratedFileCount: int32(migratedFileCount)}, nil

}

func (nws *NetworkVideoContentService) RemoveNode(ctx context.Context, rr *pb.RemoveNodeRequest) (*pb.RemoveNodeResponse, error) {

	q_ctx := context.Background() // should a new context be created?
	migratedFileCount := 0
	// get the idx, currentNode and the successorNode index from the list
	currentNodeIdx, err := nws.getNodeIndex(rr.NodeAddress)
	if err != nil {
		return nil, err
	}
	nws.mu.Lock()
	defer nws.mu.Unlock()
	if len(nws.aliveNodes) > 1 {
		currentNode, successorNode := nws.aliveNodes[currentNodeIdx], nws.aliveNodes[(currentNodeIdx+1)%len(nws.aliveNodes)]
		// move data to its successor if it exists
		data, err := currentNode.client.List(q_ctx, &pb.ListRequest{}) // TODO: can I pass the same context?
		if err == nil {
			fmt.Printf("rem node: Moving data between %s -> %s\n", currentNode.addr, successorNode.addr)
			moveDataBetweenNodes(currentNode.client, successorNode.client, data.Files)
			migratedFileCount = len(data.Files)
		} else {
			fmt.Printf("cant get list from node")
		}
	}
	// close the connection and remove node c.conn.close()
	nws.aliveNodes[currentNodeIdx].conn.Close()
	nws.aliveNodes = append(nws.aliveNodes[:currentNodeIdx], nws.aliveNodes[currentNodeIdx+1:]...)
	return &pb.RemoveNodeResponse{MigratedFileCount: int32(migratedFileCount)}, nil
}

func hashStringToUint64(s string) uint64 {
	sum := sha256.Sum256([]byte(s))
	return binary.BigEndian.Uint64(sum[:8])
}

func getDataBelongingToNode(data []string, nodeHash uint64, successorHash uint64) []string {
	// must handle the wraparound case -
	// discard the data that belongs to the successor node
	// make sure nodeHash and successorHash are distinct before calling this function
	filteredList := make([]string, 0)
	for _, file := range data {
		// fmt.Printf("file: %s\n", file)
		fileHash := hashStringToUint64(file)
		if nodeHash < successorHash {
			// non wraparound case
			if !(fileHash <= successorHash && fileHash > nodeHash) {
				// fmt.Printf("append: yes")
				filteredList = append(filteredList, file)
			}
		} else {
			// wrap around case
			if fileHash <= nodeHash && fileHash > successorHash {
				// fmt.Printf("append: yes")
				filteredList = append(filteredList, file)
			}
		}
	}
	return filteredList
}

func (nws *NetworkVideoContentService) getNodeIndex(addr string) (int, error) {

	nws.mu.RLock()
	defer nws.mu.RUnlock()
	for idx, node := range nws.aliveNodes {
		if node.addr == addr {
			return idx, nil
		}
	}
	return -1, errors.New("node not found")

}

func moveDataBetweenNodes(srcNode pb.StorageServiceClient, destNode pb.StorageServiceClient, data []string) {

	// read data from source and write to destination and remove from the source
	for idx, file := range data {
		videoID := path.Dir(file)   // "videoId"
		fileName := path.Base(file) // "file.mxx"
		fileData, err := srcNode.Read(context.Background(), &pb.ReadRequest{VideoId: videoID, FileName: fileName})
		if err != nil {
			fmt.Printf("Read GRPC failed while moving data for file %d", idx) // Does it have to be fatal?
		}
		_, err = destNode.Write(context.Background(), &pb.WriteRequest{VideoId: videoID, FileName: fileName, FileData: fileData.FileData})
		if err != nil {
			fmt.Printf("Write GRPC failed while moving data for file %d", idx) // Does it have to be fatal?
		}
		_, err = srcNode.Remove(context.Background(), &pb.RemoveRequest{VideoId: videoID, FileName: fileName})
		if err != nil {
			fmt.Printf("Remove GRPC failed while moving data for file %d", idx) // Does it have to be fatal?
		}
	}
}

func (nws *NetworkVideoContentService) getNodeForHash(hash uint64) (*node, error) {
	// fmt.Printf("Nodes alive: %d", len(nws.aliveNodes))
	//assumes a sorted list
	nws.mu.RLock()
	defer nws.mu.RUnlock()
	if len(nws.aliveNodes) == 0 {
		return nil, errors.New("no live nodes")
	}
	nodeToReturn := &nws.aliveNodes[0] // to loop around
	for _, node := range nws.aliveNodes {
		if node.hash > hash {
			nodeToReturn = &node
			break
		}
	}
	return nodeToReturn, nil
}

// Uncomment the following line to ensure NetworkVideoContentService implements VideoContentService
var _ VideoContentService = (*NetworkVideoContentService)(nil)
