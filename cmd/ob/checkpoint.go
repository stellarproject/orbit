/*
	Copyright (c) 2019 Stellar Project

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package main

import (
	v1 "github.com/stellarproject/orbit/api/orbit/v1"
	"github.com/urfave/cli"
)

var checkpointCommand = cli.Command{
	Name:  "checkpoint",
	Usage: "checkpoint a container",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "live",
			Usage: "enable live checkpoint(criu must be installed)",
		},
		cli.BoolFlag{
			Name:  "exit",
			Usage: "exit the container after a successful checkpoint",
		},
		cli.StringFlag{
			Name:  "ref",
			Usage: "ref name of the created checkpoint",
		},
		cli.BoolFlag{
			Name:  "push",
			Usage: "push the successful checkpoint",
		},
	},
	Action: func(clix *cli.Context) error {
		ctx := Context()
		agent, err := Agent(clix)
		if err != nil {
			return err
		}
		defer agent.Close()
		ref := clix.String("ref")
		if _, err := agent.Checkpoint(ctx, &v1.CheckpointRequest{
			ID:   clix.Args().First(),
			Ref:  ref,
			Live: clix.Bool("live"),
			Exit: clix.Bool("exit"),
		}); err != nil {
			return err
		}
		if clix.Bool("push") {
			_, err = agent.Push(ctx, &v1.PushRequest{
				Ref: ref,
			})
			return err
		}
		return nil
	},
}
