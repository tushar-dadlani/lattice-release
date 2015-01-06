package command_factory_test

import (
	"errors"
	"io"

	"github.com/dajulia3/cli"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	config_package "github.com/pivotal-cf-experimental/lattice-cli/config"
	"github.com/pivotal-cf-experimental/lattice-cli/config/persister"
	"github.com/pivotal-cf-experimental/lattice-cli/config/target_verifier/fake_target_verifier"
	"github.com/pivotal-cf-experimental/lattice-cli/output"
	"github.com/pivotal-cf-experimental/lattice-cli/test_helpers"

	"github.com/pivotal-cf-experimental/lattice-cli/config/command_factory"
)

var _ = Describe("CommandFactory", func() {
	var (
		stdinReader      *io.PipeReader
		stdinWriter      *io.PipeWriter
		outputBuffer     *gbytes.Buffer
		setTargetCommand cli.Command
		config           *config_package.Config
		targetVerifier   *fake_target_verifier.FakeTargetVerifier
	)

	BeforeEach(func() {
		stdinReader, stdinWriter = io.Pipe()
		outputBuffer = gbytes.NewBuffer()
		targetVerifier = &fake_target_verifier.FakeTargetVerifier{}

		config = config_package.New(persister.NewMemPersister())

		commandFactory := command_factory.NewConfigCommandFactory(config, targetVerifier, stdinReader, output.New(outputBuffer))
		setTargetCommand = commandFactory.MakeSetTargetCommand()
	})

	Describe("SetTargetCommand", func() {
		verifyOldTargetStillSet := func() {
			config.Load()
			Expect(config.Receptor()).To(Equal("http://olduser:oldpass@receptor.oldtarget.com"))
		}

		BeforeEach(func() {
			config.SetTarget("oldtarget.com")
			config.SetLogin("olduser", "oldpass")
			config.Save()
		})

		Context("target without auth", func() {
			BeforeEach(func() {
				targetVerifier.ValidateAuthorizationReturns(true, nil)
			})

			It("saves the new target", func() {
				commandFinishChan := test_helpers.AsyncExecuteCommandWithArgs(setTargetCommand, []string{"myapi.com"})

				Eventually(commandFinishChan).Should(BeClosed())

				Expect(targetVerifier.ValidateAuthorizationCallCount()).To(Equal(1))
				Expect(targetVerifier.ValidateAuthorizationArgsForCall(0)).To(Equal("http://receptor.myapi.com"))

				Expect(config.Receptor()).To(Equal("http://receptor.myapi.com"))
			})

			It("clears out existing saved target credentials", func() {
				test_helpers.ExecuteCommandWithArgs(setTargetCommand, []string{"myapi.com"})

				Expect(targetVerifier.ValidateAuthorizationCallCount()).To(Equal(1))
				Expect(targetVerifier.ValidateAuthorizationArgsForCall(0)).To(Equal("http://receptor.myapi.com"))
			})

			It("bubbles up errors from setting the target", func() {
				commandFactory := command_factory.NewConfigCommandFactory(config_package.New(errorPersister("FAILURE setting api")), targetVerifier, stdinReader, output.New(outputBuffer))
				setTargetCommand = commandFactory.MakeSetTargetCommand()

				test_helpers.ExecuteCommandWithArgs(setTargetCommand, []string{"myapi.com"})

				Eventually(outputBuffer).Should(test_helpers.Say("FAILURE setting api"))
			})
		})

		Context("target requiring auth", func() {
			BeforeEach(func() {
				targetVerifier.ValidateAuthorizationReturns(false, nil)
			})

			It("sets the api, username, password from the target specified", func() {
				commandFinishChan := test_helpers.AsyncExecuteCommandWithArgs(setTargetCommand, []string{"myapi.com"})

				Eventually(outputBuffer).Should(test_helpers.Say("Username: "))
				stdinWriter.Write([]byte("testusername\n"))
				Eventually(outputBuffer).Should(test_helpers.Say("Password: "))

				targetVerifier.ValidateAuthorizationReturns(true, nil)
				stdinWriter.Write([]byte("testpassword\n"))

				Eventually(commandFinishChan).Should(BeClosed())

				Expect(config.Target()).To(Equal("myapi.com"))
				Expect(config.Receptor()).To(Equal("http://testusername:testpassword@receptor.myapi.com"))
				Expect(outputBuffer).To(test_helpers.Say("Api Location Set"))

				Expect(targetVerifier.ValidateAuthorizationCallCount()).To(Equal(2))
				Expect(targetVerifier.ValidateAuthorizationArgsForCall(0)).To(Equal("http://receptor.myapi.com"))
				Expect(targetVerifier.ValidateAuthorizationArgsForCall(1)).To(Equal("http://testusername:testpassword@receptor.myapi.com"))
			})

			It("does not save the config if the receptor is never authorized", func() {
				commandFinishChan := test_helpers.AsyncExecuteCommandWithArgs(setTargetCommand, []string{"newtarget.com"})

				Eventually(outputBuffer).Should(test_helpers.Say("Username: "))
				stdinWriter.Write([]byte("notgood\n"))
				Eventually(outputBuffer).Should(test_helpers.Say("Password: "))
				stdinWriter.Write([]byte("evenworse\n"))

				Eventually(commandFinishChan).Should(BeClosed())
				Expect(outputBuffer).To(test_helpers.Say("Could not authorize target."))

				verifyOldTargetStillSet()
			})

			It("does not save the config if the receptor is never authorized", func() {
				commandFinishChan := test_helpers.AsyncExecuteCommandWithArgs(setTargetCommand, []string{"newtarget.com"})

				Eventually(outputBuffer).Should(test_helpers.Say("Username: "))
				stdinWriter.Write([]byte("notgood\n"))
				Eventually(outputBuffer).Should(test_helpers.Say("Password: "))

				targetVerifier.ValidateAuthorizationReturns(false, errors.New("Unknown Error"))
				stdinWriter.Write([]byte("evenworse\n"))

				Eventually(commandFinishChan).Should(BeClosed())
				Expect(outputBuffer).To(test_helpers.Say("Error verifying target: Unknown Error"))

				verifyOldTargetStillSet()
			})
		})

		Context("setting an invalid target", func() {
			It("returns an error if the target is blank", func() {
				test_helpers.ExecuteCommandWithArgs(setTargetCommand, []string{""})

				Expect(outputBuffer).To(test_helpers.Say("Incorrect Usage: Target required."))

				verifyOldTargetStillSet()
			})

			It("does not save the config if the target verifier returns an error", func() {
				targetVerifier.ValidateAuthorizationReturns(false, errors.New("Unknown Error"))

				test_helpers.ExecuteCommandWithArgs(setTargetCommand, []string{"newtarget.com"})

				Expect(outputBuffer).To(test_helpers.Say("Error verifying target: Unknown Error"))

				verifyOldTargetStillSet()
			})
		})
	})
})

type errorPersister string

func (f errorPersister) Load(i interface{}) error {
	return errors.New(string(f))
}

func (f errorPersister) Save(i interface{}) error {
	return errors.New(string(f))
}
