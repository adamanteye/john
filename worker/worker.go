package worker

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/magnusfurugard/multi-john/worker/john"
	"github.com/magnusfurugard/multi-john/worker/node"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

func New(logger *zap.Logger, cli *clientv3.Client, johnFile string, johnFlags string) error {
	sugar := logger.Sugar()

	// Cofigure node
	var totalNodes int
	if n, ok := os.LookupEnv("TOTAL_NODES"); ok {
		totalNodes, _ = strconv.Atoi(n)
	} else {
		sugar.Warn("TOTAL_NODES environment missing, defaulting to TOTAL_NODES=2")
		totalNodes = 2
	}

	n, err := node.New(totalNodes, cli, logger)
	if err != nil {
		sugar.Errorf("Unable to start node: %v", err)
		cli.Close()
		return err
	}

	// Configure john
	flags := map[string]string{}
	fl := strings.Split(johnFlags, ",")
	if len(fl[0]) > 0 {
		for _, flag := range fl {
			f := strings.SplitN(flag, "=", 2)
			value := ""
			if len(f) == 2 {
				value = f[1]
			}
			flags[f[0]] = value
			sugar.Info(flags)
		}
	}
	flags["--node"] = fmt.Sprintf("%v/%v", n.Number, n.TotalNodes)

	var johnPath string
	if j, ok := os.LookupEnv("JOHN_PATH"); ok {
		johnPath = j
	} else {
		johnPath = "john"
	}

	cmd := john.New(
		johnPath,
		johnFile,
		flags,
		logger,
	)

	if err := n.Start(cmd); err != nil {
		return err
	}
	return nil
}
