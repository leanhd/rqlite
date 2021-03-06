package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/Bowery/prompt"
	"github.com/mkideal/cli"
)

const maxRedirect = 21

type argT struct {
	cli.Helper
	Protocol string `cli:"s,scheme" usage:"protocol scheme (http or https)" dft:"http"`
	Host     string `cli:"H,host" usage:"rqlited host address" dft:"127.0.0.1"`
	Port     uint16 `cli:"p,port" usage:"rqlited host port" dft:"4001"`
	Prefix   string `cli:"P,prefix" usage:"rqlited HTTP URL prefix" dft:"/"`
	Insecure bool   `cli:"i,insecure" usage:"do not verify rqlited HTTPS certificate" dft:"false"`
}

func main() {
	cli.SetUsageStyle(cli.ManualStyle)
	cli.Run(new(argT), func(ctx *cli.Context) error {
		argv := ctx.Argv().(*argT)
		if argv.Help {
			ctx.WriteUsage()
			return nil
		}

		prefix := fmt.Sprintf("%s:%d> ", argv.Host, argv.Port)
	FOR_READ:
		for {
			line, err := prompt.Basic(prefix, false)
			if err != nil {
				return err
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var (
				index = strings.Index(line, " ")
				cmd   = line
			)
			if index >= 0 {
				cmd = line[:index]
			}
			cmd = strings.ToUpper(cmd)
			switch cmd {
			case ".TABLES":
				err = query(ctx, cmd, `SELECT name FROM sqlite_master WHERE type="table"`, argv)
			case ".INDEXES":
				err = query(ctx, cmd, `SELECT sql FROM sqlite_master WHERE type="index"`, argv)
			case ".SCHEMA":
				err = query(ctx, cmd, "SELECT sql FROM sqlite_master", argv)
			case ".QUIT", "QUIT", "EXIT":
				break FOR_READ
			case "SELECT":
				err = query(ctx, cmd, line, argv)
			default:
				err = execute(ctx, cmd, line, argv)
			}
			if err != nil {
				ctx.String("%s %v\n", ctx.Color().Red("ERR!"), err)
			}
		}
		ctx.String("bye~\n")
		return nil
	})
}

func makeJSONBody(line string) string {
	data, err := json.MarshalIndent([]string{line}, "", "    ")
	if err != nil {
		return ""
	}
	return string(data)
}

func sendRequest(ctx *cli.Context, urlStr string, line string, argv *argT, ret interface{}) error {
	data := makeJSONBody(line)
	url := urlStr

	client := http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: argv.Insecure},
	}}

	// Explicitly handle redirects.
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	nRedirect := 0
	for {
		resp, err := client.Post(url, "application/json", strings.NewReader(data))
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		// Check for redirect.
		if resp.StatusCode == http.StatusMovedPermanently {
			nRedirect++
			if nRedirect > maxRedirect {
				return fmt.Errorf("maximum leader redirect limit exceeded")
			}
			url = resp.Header["Location"][0]
			continue
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		if err := json.Unmarshal(body, ret); err != nil {
			return err
		}
		return nil
	}
}
