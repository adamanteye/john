package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/magnusfurugard/multi-john/worker/john"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

func New(logger *zap.Logger, cli *clientv3.Client, johnFile string, johnFlags string) error {
	var totalNodes int
	if n, ok := os.LookupEnv("TOTAL_NODES"); ok {
		var err error
		totalNodes, err = strconv.Atoi(n)
		if err != nil || totalNodes < 1 {
			return fmt.Errorf("invalid TOTAL_NODES %q", n)
		}
	} else {
		logger.Sugar().Warn("TOTAL_NODES environment missing, defaulting to TOTAL_NODES=2")
		totalNodes = 2
	}

	flags := parseFlags(johnFlags)
	johnPath := "john"
	if j, ok := os.LookupEnv("JOHN_PATH"); ok {
		johnPath = j
	}

	runID, ok := os.LookupEnv("MULTI_JOHN_RUN_ID")
	if !ok || runID == "" {
		return fmt.Errorf("worker requires MULTI_JOHN_RUN_ID")
	}
	index, err := completionIndex()
	if err != nil {
		return err
	}
	return runIndexed(logger, cli, johnPath, johnFile, flags, runID, index+1, totalNodes)
}

func parseFlags(johnFlags string) map[string]string {
	flags := map[string]string{}
	fl := strings.Split(johnFlags, ",")
	if len(fl[0]) == 0 {
		return flags
	}
	for _, flag := range fl {
		f := strings.SplitN(strings.TrimSpace(flag), "=", 2)
		if f[0] == "" {
			continue
		}
		value := ""
		if len(f) == 2 {
			value = f[1]
		}
		flags[f[0]] = value
	}
	return flags
}

func completionIndex() (int, error) {
	for _, key := range []string{"JOB_COMPLETION_INDEX", "MULTI_JOHN_NODE_INDEX"} {
		if value, ok := os.LookupEnv(key); ok && value != "" {
			index, err := strconv.Atoi(value)
			if err != nil || index < 0 {
				return 0, fmt.Errorf("invalid %s %q", key, value)
			}
			return index, nil
		}
	}
	return 0, fmt.Errorf("indexed worker missing JOB_COMPLETION_INDEX")
}

func runIndexed(logger *zap.Logger, cli *clientv3.Client, johnPath, johnFile string, flags map[string]string, runID string, nodeNumber, totalNodes int) error {
	sugar := logger.Sugar()
	flags["--node"] = fmt.Sprintf("%v/%v", nodeNumber, totalNodes)

	cmd := john.New(johnPath, johnFile, flags, logger)
	statusPath := fmt.Sprintf("runs/%s/nodes/%d/status", runID, nodeNumber)
	resultsPath := fmt.Sprintf("runs/%s/nodes/%d/results", runID, nodeNumber)

	if _, err := cli.KV.Put(context.TODO(), statusPath, "running"); err != nil {
		return err
	}

	go func() {
		for msgs := range cmd.Results {
			found, err := json.Marshal(msgs)
			if err != nil {
				sugar.Error(err)
				continue
			}
			if _, err := cli.KV.Put(context.TODO(), resultsPath, string(found)); err != nil {
				sugar.Error(err)
			}
		}
	}()

	sugar.Infof("starting indexed worker run=%s node=%d/%d", runID, nodeNumber, totalNodes)
	if err := cmd.Run(); err != nil {
		_, _ = cli.KV.Put(context.TODO(), statusPath, "failed: "+err.Error())
		return err
	}
	_, err := cli.KV.Put(context.TODO(), statusPath, "completed")
	return err
}
