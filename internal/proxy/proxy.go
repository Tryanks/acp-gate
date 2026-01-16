package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"acp-gate/internal/acpinspect"
	"acp-gate/internal/audit"
	acp "github.com/coder/acp-go-sdk"
)

// ProxyAgent implements acp.Agent, acp.AgentLoader, and acp.AgentExperimental.
// It receives calls from the upstream editor and forwards them to the downstream real agent.
type ProxyAgent struct {
	downstream acp.Agent
	store      *audit.Store
}

func NewProxyAgent(downstream acp.Agent, store *audit.Store) *ProxyAgent {
	return &ProxyAgent{downstream: downstream, store: store}
}

func (a *ProxyAgent) SetDownstream(downstream acp.Agent) {
	a.downstream = downstream
}

func (a *ProxyAgent) SetStore(store *audit.Store) {
	a.store = store
}

func (a *ProxyAgent) audit(ctx context.Context, method string, params interface{}, result interface{}, err error) {
	if a.store == nil {
		return
	}
	// Client -> Agent (Upstream to Downstream)
	rawParams, _ := json.Marshal(params)
	var rawResult json.RawMessage
	if result != nil {
		rawResult, _ = json.Marshal(result)
	}

	anyMsg := acpinspect.AnyMessage{
		Method: method,
		Params: rawParams,
	}
	sid, _, userText, agentText := acpinspect.Extract(anyMsg)

	record := audit.Record{
		Timestamp: time.Now(),
		Direction: audit.DirectionUpstreamToDownstream,
		SessionID: sid,
		Method:    method,
		IsRequest: true,
		Raw:       rawParams,
		UserText:  userText,
		AgentText: agentText,
	}
	_ = a.store.Write(ctx, record)

	if result != nil {
		respRecord := audit.Record{
			Timestamp: time.Now(),
			Direction: audit.DirectionDownstreamToUpstream,
			SessionID: sid,
			Method:    method,
			IsRequest: false,
			Raw:       rawResult,
		}
		_ = a.store.Write(ctx, respRecord)
	}
}

func (a *ProxyAgent) Initialize(ctx context.Context, req acp.InitializeRequest) (acp.InitializeResponse, error) {
	res, err := a.downstream.Initialize(ctx, req)
	a.audit(ctx, acp.AgentMethodInitialize, req, res, err)
	return res, err
}

func (a *ProxyAgent) Authenticate(ctx context.Context, req acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	res, err := a.downstream.Authenticate(ctx, req)
	a.audit(ctx, acp.AgentMethodAuthenticate, req, res, err)
	return res, err
}

func (a *ProxyAgent) NewSession(ctx context.Context, req acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	res, err := a.downstream.NewSession(ctx, req)
	a.audit(ctx, acp.AgentMethodSessionNew, req, res, err)
	return res, err
}

func (a *ProxyAgent) Prompt(ctx context.Context, req acp.PromptRequest) (acp.PromptResponse, error) {
	res, err := a.downstream.Prompt(ctx, req)
	a.audit(ctx, acp.AgentMethodSessionPrompt, req, res, err)
	return res, err
}

func (a *ProxyAgent) Cancel(ctx context.Context, req acp.CancelNotification) error {
	err := a.downstream.Cancel(ctx, req)
	a.audit(ctx, acp.AgentMethodSessionCancel, req, nil, err)
	return err
}

func (a *ProxyAgent) SetSessionMode(ctx context.Context, req acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	res, err := a.downstream.SetSessionMode(ctx, req)
	a.audit(ctx, acp.AgentMethodSessionSetMode, req, res, err)
	return res, err
}

// LoadSession implements acp.AgentLoader.
func (a *ProxyAgent) LoadSession(ctx context.Context, req acp.LoadSessionRequest) (acp.LoadSessionResponse, error) {
	if loader, ok := a.downstream.(acp.AgentLoader); ok {
		res, err := loader.LoadSession(ctx, req)
		a.audit(ctx, acp.AgentMethodSessionLoad, req, res, err)
		return res, err
	}
	return acp.LoadSessionResponse{}, fmt.Errorf("downstream does not support LoadSession")
}

// SetSessionModel implements acp.AgentExperimental.
func (a *ProxyAgent) SetSessionModel(ctx context.Context, req acp.SetSessionModelRequest) (acp.SetSessionModelResponse, error) {
	if exp, ok := a.downstream.(acp.AgentExperimental); ok {
		res, err := exp.SetSessionModel(ctx, req)
		a.audit(ctx, acp.AgentMethodSessionSetModel, req, res, err)
		return res, err
	}
	return acp.SetSessionModelResponse{}, fmt.Errorf("downstream does not support SetSessionModel")
}

// ProxyClient implements acp.Client.
// It receives calls from the downstream real agent and forwards them to the upstream editor.
type ProxyClient struct {
	upstream acp.Client
	store    *audit.Store
}

func NewProxyClient(upstream acp.Client, store *audit.Store) *ProxyClient {
	return &ProxyClient{upstream: upstream, store: store}
}

func (c *ProxyClient) SetUpstream(upstream acp.Client) {
	c.upstream = upstream
}

func (c *ProxyClient) SetStore(store *audit.Store) {
	c.store = store
}

func (c *ProxyClient) audit(ctx context.Context, method string, params interface{}, result interface{}, err error) {
	if c.store == nil {
		return
	}
	// Agent -> Client (Downstream to Upstream)
	rawParams, _ := json.Marshal(params)
	var rawResult json.RawMessage
	if result != nil {
		rawResult, _ = json.Marshal(result)
	}

	anyMsg := acpinspect.AnyMessage{
		Method: method,
		Params: rawParams,
	}
	sid, _, userText, agentText := acpinspect.Extract(anyMsg)

	isNotify := (method == acp.ClientMethodSessionUpdate)

	record := audit.Record{
		Timestamp: time.Now(),
		Direction: audit.DirectionDownstreamToUpstream,
		SessionID: sid,
		Method:    method,
		IsRequest: !isNotify,
		IsNotify:  isNotify,
		Raw:       rawParams,
		UserText:  userText,
		AgentText: agentText,
	}
	_ = c.store.Write(ctx, record)

	if result != nil {
		respRecord := audit.Record{
			Timestamp: time.Now(),
			Direction: audit.DirectionUpstreamToDownstream,
			SessionID: sid,
			Method:    method,
			IsRequest: false,
			Raw:       rawResult,
		}
		_ = c.store.Write(ctx, respRecord)
	}
}

func (c *ProxyClient) ReadTextFile(ctx context.Context, req acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	res, err := c.upstream.ReadTextFile(ctx, req)
	c.audit(ctx, acp.ClientMethodFsReadTextFile, req, res, err)
	return res, err
}

func (c *ProxyClient) WriteTextFile(ctx context.Context, req acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	res, err := c.upstream.WriteTextFile(ctx, req)
	c.audit(ctx, acp.ClientMethodFsWriteTextFile, req, res, err)
	return res, err
}

func (c *ProxyClient) CreateTerminal(ctx context.Context, req acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	res, err := c.upstream.CreateTerminal(ctx, req)
	c.audit(ctx, acp.ClientMethodTerminalCreate, req, res, err)
	return res, err
}

func (c *ProxyClient) KillTerminalCommand(ctx context.Context, req acp.KillTerminalCommandRequest) (acp.KillTerminalCommandResponse, error) {
	res, err := c.upstream.KillTerminalCommand(ctx, req)
	c.audit(ctx, acp.ClientMethodTerminalKill, req, res, err)
	return res, err
}

func (c *ProxyClient) TerminalOutput(ctx context.Context, req acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	res, err := c.upstream.TerminalOutput(ctx, req)
	c.audit(ctx, acp.ClientMethodTerminalOutput, req, res, err)
	return res, err
}

func (c *ProxyClient) ReleaseTerminal(ctx context.Context, req acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	res, err := c.upstream.ReleaseTerminal(ctx, req)
	c.audit(ctx, acp.ClientMethodTerminalRelease, req, res, err)
	return res, err
}

func (c *ProxyClient) WaitForTerminalExit(ctx context.Context, req acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	res, err := c.upstream.WaitForTerminalExit(ctx, req)
	c.audit(ctx, acp.ClientMethodTerminalWaitForExit, req, res, err)
	return res, err
}

func (c *ProxyClient) RequestPermission(ctx context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	res, err := c.upstream.RequestPermission(ctx, req)
	c.audit(ctx, acp.ClientMethodSessionRequestPermission, req, res, err)
	return res, err
}

func (c *ProxyClient) SessionUpdate(ctx context.Context, req acp.SessionNotification) error {
	err := c.upstream.SessionUpdate(ctx, req)
	c.audit(ctx, acp.ClientMethodSessionUpdate, req, nil, err)
	return err
}
