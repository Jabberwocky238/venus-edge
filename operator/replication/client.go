package replication

import (
	"context"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	conn   *grpc.ClientConn
	client ReplicationServiceClient
}

func Dial(ctx context.Context, target string, opts ...grpc.DialOption) (*Client, error) {
	if len(opts) == 0 {
		opts = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	}
	conn, err := grpc.DialContext(ctx, target, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn:   conn,
		client: NewReplicationServiceClient(conn),
	}, nil
}

func NewClient(conn grpc.ClientConnInterface) *Client {
	return &Client{client: NewReplicationServiceClient(conn)}
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) Subscribe(ctx context.Context, podIP, agentID string, versionIndex uint64, opts ...grpc.CallOption) (ReplicationService_SubscribeClient, error) {
	return c.client.Subscribe(ctx, &PushChangeRequest{
		PodIp:        podIP,
		AgentId:      agentID,
		VersionIndex: versionIndex,
	}, opts...)
}

func RecvChange(stream ReplicationService_SubscribeClient) (*ChangeEnvelope, error) {
	if stream == nil {
		return nil, io.EOF
	}
	return stream.Recv()
}
