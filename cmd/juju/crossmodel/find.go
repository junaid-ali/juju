// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
)

const findCommandDoc = `
Find which offered application endpoints are available to the current user.

This command is aimed for a user who wants to discover what endpoints are available to them.

options:
-o, --output (= "")
   specify an output file
--format (= tabular)
   specify output format (tabular|json|yaml)

Examples:
   $ juju find-endpoints
   $ juju find-endpoints mycontroller:
   $ juju find-endpoints fred/prod
   $ juju find-endpoints --interface mysql --url fred/prod
   $ juju find-endpoints --url fred/prod.db2
   
See also:
   show-endpoints   
`

type findCommand struct {
	RemoteEndpointsCommandBase

	url            string
	modelOwnerName string
	modelName      string
	offerName      string
	interfaceName  string
	endpoint       string

	out        cmd.Output
	newAPIFunc func() (FindAPI, error)
}

// NewFindEndpointsCommand constructs command that
// allows to find offered application endpoints.
func NewFindEndpointsCommand() cmd.Command {
	findCmd := &findCommand{}
	findCmd.newAPIFunc = func() (FindAPI, error) {
		return findCmd.NewRemoteEndpointsAPI()
	}
	return modelcmd.WrapController(findCmd)
}

// Init implements Command.Init.
func (c *findCommand) Init(args []string) (err error) {
	url, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return errors.Trace(err)
	}
	if url != "" {
		if c.url != "" {
			return errors.New("URL term cannot be specified twice")
		}
		c.url = url
	}
	return nil
}

// Info implements Command.Info.
func (c *findCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "find-endpoints",
		Purpose: "Find offered application endpoints",
		Doc:     findCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *findCommand) SetFlags(f *gnuflag.FlagSet) {
	c.RemoteEndpointsCommandBase.SetFlags(f)
	f.StringVar(&c.url, "url", "", "application URL")
	f.StringVar(&c.interfaceName, "interface", "", "return results matching the interface name")
	f.StringVar(&c.endpoint, "endpoint", "", "return results matching the endpoint name")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatFindTabular,
	})
}

// Run implements Command.Run.
func (c *findCommand) Run(ctx *cmd.Context) (err error) {
	if err := c.validateOrSetURL(); err != nil {
		return errors.Trace(err)
	}
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	filter := crossmodel.ApplicationOfferFilter{
		OwnerName: c.modelOwnerName,
		ModelName: c.modelName,
		OfferName: c.offerName,
		// TODO(wallyworld): interface
		// TODO(wallyworld): endpoint
	}
	if c.interfaceName != "" || c.endpoint != "" {
		filter.Endpoints = []crossmodel.EndpointFilterTerm{{
			Interface: c.interfaceName,
			Name:      c.endpoint,
		}}
	}
	found, err := api.FindApplicationOffers(filter)
	if err != nil {
		return err
	}

	output, err := convertFoundOffers(found...)
	if err != nil {
		return err
	}
	if len(output) == 0 {
		return errors.New("no matching application offers found")
	}
	return c.out.Write(ctx, output)
}

func (c *findCommand) validateOrSetURL() error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	if c.url == "" {
		c.url = controllerName + ":"
		return nil
	}
	urlParts, err := crossmodel.ParseApplicationURLParts(c.url)
	if err != nil {
		return errors.Trace(err)
	}
	user := urlParts.User
	if user == "" {
		accountDetails, err := c.CurrentAccountDetails()
		if err != nil {
			return errors.Trace(err)
		}
		user = accountDetails.User
	}
	c.modelOwnerName = user
	c.modelName = urlParts.ModelName
	c.offerName = urlParts.ApplicationName
	if urlParts.Source != "" && urlParts.Source != controllerName {
		return errors.NotSupportedf("finding endpoints from another controller %q", urlParts.Source)
	}
	return nil
}

// FindAPI defines the API methods that cross model find command uses.
type FindAPI interface {
	Close() error
	FindApplicationOffers(filters ...crossmodel.ApplicationOfferFilter) ([]params.ApplicationOffer, error)
}

// ApplicationOfferResult defines the serialization behaviour of an application offer.
// This is used in map-style yaml output where remote application URL is the key.
type ApplicationOfferResult struct {
	// Access is the level of access the user has on the offer.
	Access string `yaml:"access" json:"access"`

	// Endpoints is the list of offered application endpoints.
	Endpoints map[string]RemoteEndpoint `yaml:"endpoints" json:"endpoints"`
}

// convertFoundOffers takes any number of api-formatted remote applications and
// creates a collection of ui-formatted applications.
func convertFoundOffers(services ...params.ApplicationOffer) (map[string]ApplicationOfferResult, error) {
	if len(services) == 0 {
		return nil, nil
	}
	output := make(map[string]ApplicationOfferResult, len(services))
	for _, one := range services {
		app := ApplicationOfferResult{
			Access:    one.Access,
			Endpoints: convertRemoteEndpoints(one.Endpoints...),
		}
		output[one.OfferURL] = app
	}
	return output, nil
}
