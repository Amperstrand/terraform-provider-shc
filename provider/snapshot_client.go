package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (c *SHCClient) CreateSnapshot(ctx context.Context, serviceID, name string) (*SnapshotResponse, error) {
	return retryOnLockValue(ctx, func() (*SnapshotResponse, error) {
		return c.createSnapshotOnce(ctx, serviceID, name)
	})
}

func (c *SHCClient) createSnapshotOnce(ctx context.Context, serviceID, name string) (*SnapshotResponse, error) {
	path := "/vm/" + serviceID + "/snapshots"

	body, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return nil, fmt.Errorf("marshaling snapshot request: %w", err)
	}

	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusConflict {
		respBody, err = c.handleConfirmation(ctx, http.MethodPost, path, body, respBody)
		if err != nil {
			return nil, err
		}
	} else if statusCode >= 400 {
		return nil, fmt.Errorf("create snapshot failed (status %d): %s", statusCode, string(respBody))
	}

	var snapResp SnapshotResponse
	if err := json.Unmarshal(respBody, &snapResp); err != nil {
		return nil, fmt.Errorf("parsing snapshot response: %w (body: %s)", err, string(respBody))
	}

	if snapResp.ID.String() == "" {
		for attempt := 0; attempt < 30; attempt++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
			}
			snapshots, err := c.GetSnapshots(ctx, serviceID)
			if err != nil {
				continue
			}
			for _, s := range snapshots {
				if s.Name == name || (name == "" && s.ID.String() != "") {
					return &s, nil
				}
			}
		}
		return nil, fmt.Errorf("snapshot '%s' not found after polling", name)
	}

	return &snapResp, nil
}

func (c *SHCClient) GetSnapshots(ctx context.Context, serviceID string) ([]SnapshotResponse, error) {
	path := "/vm/" + serviceID + "/snapshots"

	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusNotFound {
		return nil, nil
	}

	if statusCode >= 400 {
		return nil, fmt.Errorf("get snapshots failed (status %d): %s", statusCode, string(respBody))
	}

	unwrapped := unwrapData(respBody)

	var snaps []SnapshotResponse
	if err := json.Unmarshal(unwrapped, &snaps); err == nil {
		return snaps, nil
	}

	var wrapped struct {
		Snapshots []SnapshotResponse `json:"snapshots"`
	}
	if err := json.Unmarshal(unwrapped, &wrapped); err == nil && wrapped.Snapshots != nil {
		return wrapped.Snapshots, nil
	}

	var items struct {
		Items []SnapshotResponse `json:"items"`
	}
	if err := json.Unmarshal(unwrapped, &items); err == nil && items.Items != nil {
		return items.Items, nil
	}

	return nil, fmt.Errorf("unable to parse snapshots response: %s", string(respBody))
}

func (c *SHCClient) DeleteSnapshot(ctx context.Context, serviceID, snapshotID string) error {
	return retryOnLock(ctx, func() error {
		return c.deleteSnapshotOnce(ctx, serviceID, snapshotID)
	})
}

func (c *SHCClient) deleteSnapshotOnce(ctx context.Context, serviceID, snapshotID string) error {
	path := "/vm/" + serviceID + "/snapshots/delete"

	body, err := json.Marshal(map[string]string{"snapshot_id": snapshotID})
	if err != nil {
		return fmt.Errorf("marshaling delete snapshot request: %w", err)
	}

	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return err
	}

	if statusCode == http.StatusConflict {
		_, err = c.handleConfirmation(ctx, http.MethodPost, path, body, respBody)
		return err
	}

	if statusCode == http.StatusNotFound {
		return nil
	}

	if statusCode >= 400 {
		return fmt.Errorf("delete snapshot failed (status %d): %s", statusCode, string(respBody))
	}

	return nil
}

func (c *SHCClient) CreateBackup(ctx context.Context, serviceID, name string) (*BackupResponse, error) {
	return retryOnLockValue(ctx, func() (*BackupResponse, error) {
		return c.createBackupOnce(ctx, serviceID, name)
	})
}

func (c *SHCClient) createBackupOnce(ctx context.Context, serviceID, name string) (*BackupResponse, error) {
	path := "/vm/" + serviceID + "/backups"

	body, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return nil, fmt.Errorf("marshaling backup request: %w", err)
	}

	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusConflict {
		respBody, err = c.handleConfirmation(ctx, http.MethodPost, path, body, respBody)
		if err != nil {
			return nil, err
		}
	} else if statusCode >= 400 {
		return nil, fmt.Errorf("create backup failed (status %d): %s", statusCode, string(respBody))
	}

	var backupResp BackupResponse
	if err := json.Unmarshal(respBody, &backupResp); err != nil {
		return nil, fmt.Errorf("parsing backup response: %w (body: %s)", err, string(respBody))
	}

	return &backupResp, nil
}

func (c *SHCClient) GetBackups(ctx context.Context, serviceID string) ([]BackupResponse, error) {
	path := "/vm/" + serviceID + "/backups"

	statusCode, respBody, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusNotFound {
		return nil, nil
	}

	if statusCode >= 400 {
		return nil, fmt.Errorf("get backups failed (status %d): %s", statusCode, string(respBody))
	}

	unwrapped := unwrapData(respBody)

	var backups []BackupResponse
	if err := json.Unmarshal(unwrapped, &backups); err == nil {
		return backups, nil
	}

	var wrapped struct {
		Backups []BackupResponse `json:"backups"`
	}
	if err := json.Unmarshal(unwrapped, &wrapped); err == nil && wrapped.Backups != nil {
		return wrapped.Backups, nil
	}

	var items struct {
		Items []BackupResponse `json:"items"`
	}
	if err := json.Unmarshal(unwrapped, &items); err == nil && items.Items != nil {
		return items.Items, nil
	}

	return nil, fmt.Errorf("unable to parse backups response: %s", string(respBody))
}

func (c *SHCClient) DeleteBackup(ctx context.Context, serviceID, backupID string) error {
	return retryOnLock(ctx, func() error {
		return c.deleteBackupOnce(ctx, serviceID, backupID)
	})
}

func (c *SHCClient) deleteBackupOnce(ctx context.Context, serviceID, backupID string) error {
	path := "/vm/" + serviceID + "/backups/delete"

	body, err := json.Marshal(map[string]string{"backup_id": backupID})
	if err != nil {
		return fmt.Errorf("marshaling delete backup request: %w", err)
	}

	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return err
	}

	if statusCode == http.StatusConflict {
		_, err = c.handleConfirmation(ctx, http.MethodPost, path, body, respBody)
		return err
	}

	if statusCode == http.StatusNotFound {
		return nil
	}

	if statusCode >= 400 {
		return fmt.Errorf("delete backup failed (status %d): %s", statusCode, string(respBody))
	}

	return nil
}

func (c *SHCClient) RestoreSnapshot(ctx context.Context, serviceID, snapshotID string) error {
	path := "/vm/" + serviceID + "/snapshots/restore"

	body, err := json.Marshal(map[string]string{"snapshot_id": snapshotID})
	if err != nil {
		return fmt.Errorf("marshaling restore snapshot request: %w", err)
	}

	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return err
	}

	if statusCode == http.StatusConflict {
		_, err = c.handleConfirmation(ctx, http.MethodPost, path, body, respBody)
		return err
	}

	if statusCode >= 400 {
		return fmt.Errorf("restore snapshot failed (status %d): %s", statusCode, string(respBody))
	}

	return nil
}

func (c *SHCClient) RestoreBackup(ctx context.Context, serviceID, backupID string) error {
	path := "/vm/" + serviceID + "/backups/restore"

	body, err := json.Marshal(map[string]string{"backup_id": backupID})
	if err != nil {
		return fmt.Errorf("marshaling restore backup request: %w", err)
	}

	statusCode, respBody, err := c.doRequest(ctx, http.MethodPost, path, body, "")
	if err != nil {
		return err
	}

	if statusCode == http.StatusConflict {
		_, err = c.handleConfirmation(ctx, http.MethodPost, path, body, respBody)
		return err
	}

	if statusCode >= 400 {
		return fmt.Errorf("restore backup failed (status %d): %s", statusCode, string(respBody))
	}

	return nil
}
