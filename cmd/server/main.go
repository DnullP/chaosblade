package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/chaosblade-io/chaosblade-spec-go/util"

	"github.com/chaosblade-io/chaosblade/data"
	"github.com/chaosblade-io/chaosblade/pkg/server/grpcapi"
	bladeserver "github.com/chaosblade-io/chaosblade/pkg/server/http"
	"github.com/chaosblade-io/chaosblade/pkg/service/dispatcher"
	"github.com/chaosblade-io/chaosblade/pkg/service/experiment"
	"github.com/chaosblade-io/chaosblade/pkg/service/preparation"
)

func main() {
	httpAddr := flag.String("http", ":9000", "HTTP listen address")
	grpcAddr := flag.String("grpc", ":9001", "gRPC listen address")
	authToken := flag.String("auth-token", "", "Bearer token for inbound requests")
	flag.Parse()

	util.InitLog(util.Blade)

	ds := data.GetSource()
	disp := dispatcher.New()
	if err := disp.LoadDefaultExecutors(); err != nil {
		log.Fatalf("load executors failed: %v", err)
	}

	expService := experiment.New(disp, ds)
	prepService := preparation.New(ds)
	ginServer := bladeserver.NewServer(expService, prepService, *authToken)

	go func() {
		if err := grpcapi.ListenAndServe(*grpcAddr, expService); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	if err := ginServer.Engine().Run(*httpAddr); err != nil {
		log.Fatalf("HTTP server failed: %v", err)
	}
	fmt.Println("servers exited")
}
