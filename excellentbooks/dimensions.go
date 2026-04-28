package excellentbooks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

const (
	registerObject     = "ObjVc"
	registerProject    = "PRVc"
	registerDepartment = "DepVc"
)

// ListObjects retrieves all cost center / dimension entries (register ObjVc).
func (c *Client) ListObjects(ctx context.Context, params ListParams) ([]Object, string, error) {
	resp, err := c.get(ctx, registerObject, params)
	if err != nil {
		return nil, "", err
	}
	var envelope struct {
		ResponseMeta
		ObjVc []Object `json:"ObjVc"`
	}
	if err := json.Unmarshal(resp.Data, &envelope); err != nil {
		slog.Error("excellentbooks: parse objects failed", "raw_data", string(resp.Data))
		return nil, "", fmt.Errorf("excellentbooks: parse objects: %w", err)
	}
	if len(envelope.ObjVc) == 0 {
		slog.Warn("excellentbooks: ObjVc returned 0 items", "raw_data", string(resp.Data))
	}
	return envelope.ObjVc, envelope.Sequence, nil
}

// ListProjects retrieves all projects (register PRVc).
func (c *Client) ListProjects(ctx context.Context, params ListParams) ([]Project, string, error) {
	resp, err := c.get(ctx, registerProject, params)
	if err != nil {
		return nil, "", err
	}
	var envelope struct {
		ResponseMeta
		PRVc []Project `json:"PRVc"`
	}
	if err := json.Unmarshal(resp.Data, &envelope); err != nil {
		slog.Error("excellentbooks: parse projects failed", "raw_data", string(resp.Data))
		return nil, "", fmt.Errorf("excellentbooks: parse projects: %w", err)
	}
	if len(envelope.PRVc) == 0 {
		slog.Warn("excellentbooks: PRVc returned 0 items", "raw_data", string(resp.Data))
	}
	return envelope.PRVc, envelope.Sequence, nil
}

// ListDepartments retrieves all departments (register DepVc).
func (c *Client) ListDepartments(ctx context.Context, params ListParams) ([]Department, string, error) {
	resp, err := c.get(ctx, registerDepartment, params)
	if err != nil {
		return nil, "", err
	}
	var envelope struct {
		ResponseMeta
		DepVc []Department `json:"DepVc"`
	}
	if err := json.Unmarshal(resp.Data, &envelope); err != nil {
		slog.Error("excellentbooks: parse departments failed", "raw_data", string(resp.Data))
		return nil, "", fmt.Errorf("excellentbooks: parse departments: %w", err)
	}
	if len(envelope.DepVc) == 0 {
		slog.Warn("excellentbooks: DepVc returned 0 items", "raw_data", string(resp.Data))
	}
	return envelope.DepVc, envelope.Sequence, nil
}
