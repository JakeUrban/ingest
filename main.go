package main

import (
	"context"
	"io"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
	"github.com/stellar/go/ingest"
	backends "github.com/stellar/go/ingest/ledgerbackend"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/xdr"
)

func main() {
	log.DefaultLogger = log.New()
	log.DefaultLogger.SetLevel(log.InfoLevel)

	archiveUrls := []string{
		"https://history.stellar.org/prd/core-testnet/core_testnet_001",
	}
	networkPassphrase := "Test SDF Network ; September 2015"

	seqVal := os.Getenv("STARTING_AT_LEDGER")
	if seqVal == "" {
		log.Error("no STARATING_AT_LEDGER env var specified")
		return
	}
	seq, err := strconv.ParseUint(seqVal, 10, 32)
	if err != nil {
		log.Error("invalid STARTING_AT_LEDGER env var")
		return
	}
	account := os.Getenv("ACCOUNT")
	if account == "" {
		log.Error("no ACCOUNT env var specified")
		return
	}

	backend := getCaptiveCore(archiveUrls, networkPassphrase)
	defer backend.Close()

	streamPayments(account, uint32(seq), networkPassphrase, backend)
}

func getCaptiveCore(archiveUrls []string, networkPassphrase string) *backends.CaptiveStellarCore {
	logger := log.New()
	logger.SetLevel(logrus.WarnLevel)

	captiveCoreToml, err := backends.NewCaptiveCoreTomlFromFile(
		"/etc/stellar/stellar-core.toml",
		backends.CaptiveCoreTomlParams{
			NetworkPassphrase:  networkPassphrase,
			HistoryArchiveURLs: archiveUrls,
		})
	panicIf(err)

	config := backends.CaptiveCoreConfig{
		// Change these based on your environment:
		BinaryPath:         "/usr/bin/stellar-core",
		NetworkPassphrase:  networkPassphrase,
		HistoryArchiveURLs: archiveUrls,
		Toml:               captiveCoreToml,
		Log:                logger,
	}

	backend, err := backends.NewCaptive(config)
	panicIf(err)
	return backend
}

func streamPayments(account string, seq uint32, networkPassphrase string, backend *backends.CaptiveStellarCore) {
	ctx := context.Background()

	err := backend.PrepareRange(ctx, backends.UnboundedRange(seq))
	panicIf(err)

	for {
		txReader, err := ingest.NewLedgerTransactionReader(
			ctx, backend, networkPassphrase, seq,
		)
		panicIf(err)
		defer txReader.Close()

		streamPaymentsForAccountAtLedger(account, txReader)
		seq += 1
	}
}

func streamPaymentsForAccountAtLedger(account string, txReader *ingest.LedgerTransactionReader) {
	for {
		ledgerTx, err := txReader.Read()
		if err == io.EOF {
			break
		}
		panicIf(err)

		if !ledgerTx.Result.Successful() {
			continue
		}

		streamPaymentsForAccountFromTransaction(account, ledgerTx)
	}
}

func streamPaymentsForAccountFromTransaction(account string, ledgerTx ingest.LedgerTransaction) {
	for _, opXdr := range ledgerTx.Envelope.Operations() {
		if opXdr.Body.Type != xdr.OperationTypePayment {
			continue
		}
		payment := opXdr.Body.PaymentOp
		dest := payment.Destination.Address()
		var source string
		if opXdr.SourceAccount == nil {
			source = ledgerTx.Envelope.SourceAccount().ToAccountId().Address()
		} else {
			source = opXdr.SourceAccount.Address()
		}
		if dest == account || source == account {
			code := payment.Asset.GetCode()
			if code == "" {
				if payment.Asset.Type == xdr.AssetTypeAssetTypeNative {
					code = "XLM"
				} else {
					code = "liquidity pool shares"
				}
			}
			log.Infof("Account %s sent %d %s to %s", source, payment.Amount, code, dest)
		}
	}
}

func panicIf(err error) {
	if err != nil {
		panic(err)
	}
}
