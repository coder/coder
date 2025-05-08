package dbpool

import "net/rpc"

type Client struct {
	rpcClient *rpc.Client
}

func NewClient(addr string) (*Client, error) {
	rpcClient, err := rpc.DialHTTP("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &Client{rpcClient: rpcClient}, nil
}

func (c *Client) GetDB() (string, error) {
	var arg int
	var reply string
	err := c.rpcClient.Call("DBPool.GetDB", &arg, &reply)
	return reply, err
}

func (c *Client) DisposeDB(dbURL string) error {
	var reply int
	return c.rpcClient.Call("DBPool.DisposeDB", &dbURL, &reply)
}

func (c *Client) Close() error {
	return c.rpcClient.Close()
}
