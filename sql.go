package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type ServerState struct {
	CDPContext context.Context
	CancelFunc context.CancelFunc
	StartNum   int
	EndNum     int
	Idx        int
	UserAgent  string
	Cookies    string
	HTTPHeader map[string]string
}

const (
	ServerName = "sql-assistant"
	Version    = "v2.0.0"
)

func main() {
	allocCtx, cancel := chromedp.NewRemoteAllocator(
		context.Background(),
		"http://localhost:9222",
	)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	mcpServer := server.NewMCPServer(ServerName, Version)

	state := &ServerState{
		CDPContext: ctx,
		CancelFunc: cancel,
	}

	mcpServer.AddTool(mcp.NewTool("StartNum",
		mcp.WithDescription("user's starting question number"),
		mcp.WithString("startProNum", mcp.Required(), mcp.Description("user's starting question number")),
		mcp.WithString("endProNum", mcp.Required(), mcp.Description("user's ending question number"))),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			startProNum := request.Params.Arguments["startProNum"].(string)
			endProNum := request.Params.Arguments["endProNum"].(string)

			start, _ := strconv.Atoi(startProNum)
			end, _ := strconv.Atoi(endProNum)

			return handleStartNum(ctx, state, start, end)
		},
	)

	mcpServer.AddTool(mcp.NewTool("GetQuestion",
		mcp.WithDescription("get question")),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleGetQuestion(ctx, state)
		},
	)

	mcpServer.AddTool(mcp.NewTool("SetCode",
		mcp.WithDescription("set code"),
		mcp.WithString("code", mcp.Required(), mcp.Description("the code will be set"))),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			code := request.Params.Arguments["code"].(string)
			return handleSetCode(ctx, state, code)
		},
	)

	mcpServer.AddTool(mcp.NewTool("SubmitCode",
		mcp.WithDescription("submit code")),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleSubmitCode(ctx, state)
		},
	)

	if err := server.ServeStdio(mcpServer); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func handleGetQuestion(ctx context.Context, state *ServerState) (*mcp.CallToolResult, error) {
	url := fmt.Sprintf("https://course.educg.net/assignment/programSQLList.jsp?proNum=%d&assignID=33144", state.Idx)
	var question string

	err := chromedp.Run(state.CDPContext,
		chromedp.Navigate(url),
		chromedp.WaitVisible(".cgProblemContentClass"),
		chromedp.Text(".cgProblemContentClass", &question),
	)
	if err != nil {
		return nil, fmt.Errorf("could not get question: %v", err)
	}

	return mcp.NewToolResultText(fmt.Sprintf("ProNum %d: %s", state.Idx, question)), nil
}

func handleSetCode(ctx context.Context, state *ServerState, code string) (*mcp.CallToolResult, error) {
	codeJSON, err := json.Marshal(code)
	if err != nil {
		return nil, fmt.Errorf("could not marshal code: %v", err)
	}

	err = chromedp.Run(state.CDPContext,
		chromedp.WaitVisible("div.CodeMirror-code"),
		chromedp.Evaluate(fmt.Sprintf(`
			var editor = document.querySelector(".CodeMirror").CodeMirror;
			editor.setValue(%s); 
			editor.refresh();
		`, codeJSON), nil),
	)
	if err != nil {
		return nil, fmt.Errorf("could not set code: %v", err)
	}

	return mcp.NewToolResultText("Code set successfully"), nil
}

func handleSubmitCode(ctx context.Context, state *ServerState) (*mcp.CallToolResult, error) {
	err := chromedp.Run(state.CDPContext,
		chromedp.Click("#SubmitSQL"),
		chromedp.Sleep(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("could not click submit button: %v", err)
	}

	state.Idx++

	if state.Idx > state.EndNum {
		return mcp.NewToolResultText("The set number of questions has been reached, and the question is finished."), nil
	}

	return mcp.NewToolResultText("Submitted successfully"), nil
}

func handleStartNum(ctx context.Context, state *ServerState, startNum, endNum int) (*mcp.CallToolResult, error) {
	state.StartNum = startNum
	state.EndNum = endNum
	state.Idx = startNum

	return mcp.NewToolResultText("Successfully set the starting question number"), nil
}
