package scenario

import (
	"context"

	"github.com/pkg/errors"
	"github.com/qlik-oss/gopherciser/action"
	"github.com/qlik-oss/gopherciser/connection"
	"github.com/qlik-oss/gopherciser/creation"
	"github.com/qlik-oss/gopherciser/session"
)

type (
	// CreateSheetSettings settings for creating a sheet
	CreateSheetSettings struct {
		ID          string `json:"id" displayname:"Sheet ID" doc-key:"createsheet.id"`
		Title       string `json:"title" displayname:"Sheet title" doc-key:"createsheet.title"`
		Description string `json:"description" displayname:"Sheet description" doc-key:"createsheet.description"`
		// todo add option to change sheet to this sheet after creation, should probably be default
	}
)

// Validate implements ActionSettings interface
func (settings CreateSheetSettings) Validate() error {
	if settings.Title == "" {
		return errors.New("title must not be empty")
	}
	return nil
}

// Execute implements ActionSettings interface
func (settings CreateSheetSettings) Execute(sessionState *session.State,
	actionState *action.State, connectionSettings *connection.ConnectionSettings, label string, reset func()) {

	if sessionState.Connection == nil || sessionState.Connection.Sense() == nil {
		actionState.AddErrors(errors.New("Not connected to a Sense environment"))
		return
	}
	uplink := sessionState.Connection.Sense()

	app := uplink.CurrentApp
	if app == nil {
		actionState.AddErrors(errors.New("Not connected to a Sense app"))
		return
	}

	metaDef := creation.StubMetaDef(settings.Title, settings.Description)
	metaDef["rank"] = 0.0 //TODO (?) Support rank

	props := map[string]interface{}{
		"qMetaDef": metaDef,
		"qInfo":    creation.StubNxInfo("sheet"),
		"cells":    []interface{}{},
	}

	err := sessionState.SendRequest(actionState, func(ctx context.Context) error {
		genObj, err := uplink.CurrentApp.Doc.CreateObjectRaw(ctx, props)
		if err != nil {
			return err
		}
		if genObj == nil {
			return errors.Errorf("creating sheet<%s> resulted in empty object", settings.ID)
		}

		if settings.ID != "" {
			return sessionState.IDMap.Add(settings.ID, genObj.GenericId, sessionState.LogEntry)
		}

		return nil
	})
	if err != nil {
		actionState.AddErrors(errors.Wrap(err, "failed to CreateSheet"))
		return
	}

	sheetList, err := uplink.CurrentApp.GetSheetList(sessionState, actionState)
	if err != nil {
		actionState.AddErrors(errors.Wrap(err, "failed to get sheetlist"))
		return
	}

	// bug in engine makes it not send the sheet list in the changed objects list. force new layout for now
	// this should be removed once fixed so we don't send multiple GetLayout for sheet list
	if err := sessionState.SendRequest(actionState, sheetList.UpdateLayout); err != nil {
		actionState.AddErrors(err)
		return
	}

	sessionState.Wait(actionState)
}
