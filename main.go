package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/playwright-community/playwright-go"
)

const (
	ServerName = "sql-assistant"
	Version    = "v1.0.0"
)

type ServerState struct {
	Browser    playwright.Browser
	Context    playwright.BrowserContext
	Page       playwright.Page
	StartNum   int
	EndNum     int
	Idx        int
	UserAgent  string
	Cookies    string
	HTTPHeader map[string]string
}

func main() {
	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("could not launch playwright: %v", err)
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(false),
	})
	if err != nil {
		log.Fatalf("could not launch browser: %v", err)
	}

	mcpServer := server.NewMCPServer(ServerName, Version)

	state := &ServerState{Browser: browser}

	mcpServer.AddTool(mcp.NewTool("Login",
		mcp.WithDescription("start login")),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleLogin(ctx, state, browser)
		},
	)

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

func handleLogin(ctx context.Context, state *ServerState, browser playwright.Browser) (*mcp.CallToolResult, error) {
	loginPage, err := browser.NewPage()
	if err != nil {
		return nil, fmt.Errorf("could not create login page: %v", err)
	}
	defer loginPage.Close()

	_, err = loginPage.Goto("https://course.educg.net/indexcs/simple.jsp?loginErr=0", playwright.PageGotoOptions{
		Timeout:   playwright.Float(0),
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	})
	if err != nil {
		return nil, fmt.Errorf("could not open login page: %v", err)
	}

	loginPage.WaitForTimeout(15000)

	cookies, err := loginPage.Context().Cookies()
	if err != nil {
		return nil, fmt.Errorf("could not get cookies: %v", err)
	}
	var cookieStrings []string
	for _, cookie := range cookies {
		cookieStrings = append(cookieStrings, fmt.Sprintf("%s=%s", cookie.Name, cookie.Value))
	}
	state.Cookies = strings.Join(cookieStrings, "; ")

	userAgent, err := loginPage.Evaluate("() => navigator.userAgent")
	if err != nil {
		return nil, fmt.Errorf("could not get user agent: %v", err)
	}
	state.UserAgent = userAgent.(string)

	state.HTTPHeader = map[string]string{
		"User-Agent": state.UserAgent,
		"Cookie":     state.Cookies,
	}

	return mcp.NewToolResultText("登录成功，Cookie 和 User-Agent 已设置"), nil
}

func handleGetQuestion(ctx context.Context, state *ServerState) (*mcp.CallToolResult, error) {
	var err error
	state.Context, err = state.Browser.NewContext()
	if err != nil {
		log.Fatalf("could not create context: %v", err)
	}

	state.Context.SetExtraHTTPHeaders(map[string]string{
		"User-Agent": state.UserAgent,
		"Cookie":     state.Cookies,
	})
	page, err := state.Context.NewPage()
	if err != nil {
		return nil, fmt.Errorf("could not create new page: %v", err)
	}

	state.Page = page

	url := fmt.Sprintf("https://course.educg.net/assignment/programSQLList.jsp?proNum=%d&assignID=33144", state.Idx)
	_, err = page.Goto(url, playwright.PageGotoOptions{
		Timeout:   playwright.Float(0),
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	})
	if err != nil {
		return nil, fmt.Errorf("could not go to page: %v", err)
	}

	questionElement := page.Locator(".cgProblemContentClass")
	question, err := questionElement.InnerText()
	if err != nil {
		return nil, fmt.Errorf("could not get question: %v", err)
	}

	return mcp.NewToolResultText(fmt.Sprintf("ProNum %d: %s", state.Idx, question)), nil
}

func handleSetCode(ctx context.Context, state *ServerState, code string) (*mcp.CallToolResult, error) {
	_, err := state.Page.WaitForSelector("div.CodeMirror-code")
	if err != nil {
		return nil, fmt.Errorf("could not wait for editor: %v", err)
	}

	codeJSON, err := json.Marshal(code)
	if err != nil {
		return nil, fmt.Errorf("could not marshal code: %v", err)
	}

	_, err = state.Page.Evaluate(fmt.Sprintf(`
       var editor = document.querySelector(".CodeMirror").CodeMirror;
       editor.setValue(%s); editor.refresh();
   `, codeJSON))
	if err != nil {
		return nil, fmt.Errorf("could not set code: %v", err)
	}

	return mcp.NewToolResultText("Code set successfully"), nil
}

func handleSubmitCode(ctx context.Context, state *ServerState) (*mcp.CallToolResult, error) {
	defer state.Page.Close()
	submitButton := state.Page.Locator("#SubmitSQL")
	err := submitButton.Click()
	if err != nil {
		return nil, fmt.Errorf("could not click submit button: %v", err)
	}

	state.Idx++

	time.Sleep(time.Second * 2)
	if state.Idx > state.EndNum {
		return mcp.NewToolResultText("已做到设置的截至题号，做题结束"), nil
	}

	return mcp.NewToolResultText("提交成功"), nil
}

func handleStartNum(ctx context.Context, state *ServerState, startNum, endNum int) (*mcp.CallToolResult, error) {
	state.StartNum = startNum
	state.EndNum = endNum
	state.Idx = startNum

	return mcp.NewToolResultText("成功设置起始题号"), nil
}
