package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	_ "time/tzdata"

	_ "github.com/breml/rootcerts"
	"github.com/labstack/echo/v4"
	"github.com/spf13/pflag"
	"go.uber.org/fx"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database"
	"librevita.org/internal/core/database/fle"
	"librevita.org/internal/core/keystore"
	"librevita.org/internal/core/meta"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/storage"
	"librevita.org/internal/core/telemetry"
	"librevita.org/internal/database/record"
	"librevita.org/internal/database/record/clinic"
	"librevita.org/internal/database/record/user"
	clinicdomain "librevita.org/internal/domain/clinic"
	"librevita.org/internal/domain/identifier"
	patientdomain "librevita.org/internal/domain/patient"
	patientmodel "librevita.org/internal/domain/patient/model"
	patientusecase "librevita.org/internal/domain/patient/usecase"
	userdomain "librevita.org/internal/domain/user"
	userusecase "librevita.org/internal/domain/user/usecase"
	"librevita.org/pkg/errors"
	"librevita.org/pkg/ident"
	"librevita.org/pkg/log"
)

var testPatients = []patientmodel.PatientInput{
	{
		DisplayName: "Carlos Schmidt",
		BirthDate:   "1985-03-14",
		Sex:         patientmodel.SexMale,
		Phone:       "11987654321",
		Email:       "carlos.schmidt@example.com",
		Street:      "Rua Augusta, 100",
		City:        "São Paulo",
		State:       "SP",
		PostalCode:  "01305-000",
		Notes:       "Paciente de ascendência germânica para teste fonético (Shimit, Schmit)",
	},
	{
		DisplayName: "Valter Silva",
		BirthDate:   "1990-07-22",
		Sex:         patientmodel.SexMale,
		Phone:       "11987654322",
		Email:       "valter.silva@example.com",
		Street:      "Av Paulista, 1500",
		City:        "São Paulo",
		State:       "SP",
		PostalCode:  "01311-200",
		Notes:       "Teste fonético V/W (Walter -> Valter)",
	},
	{
		DisplayName: "Rossi Chiari",
		BirthDate:   "1978-11-05",
		Sex:         patientmodel.SexFemale,
		Phone:       "11987654323",
		Email:       "rossi.chiari@example.com",
		Street:      "Rua Treze de Maio, 450",
		City:        "São Paulo",
		State:       "SP",
		PostalCode:  "01327-000",
		Notes:       "Teste fonético italiano Beider-Morse (Kiari -> Chiari, Rosi -> Rossi)",
	},
	{
		DisplayName: "Ana Souza",
		BirthDate:   "1995-01-30",
		Sex:         patientmodel.SexFemale,
		Phone:       "11987654324",
		Email:       "ana.souza@example.com",
		Street:      "Rua Oscar Freire, 800",
		City:        "São Paulo",
		State:       "SP",
		PostalCode:  "01426-001",
		Notes:       "Teste fonético Z/S no português (Sousa -> Souza)",
	},
	{
		DisplayName: "Luiz Santos",
		BirthDate:   "1982-09-18",
		Sex:         patientmodel.SexMale,
		Phone:       "11987654325",
		Email:       "luiz.santos@example.com",
		Street:      "Rua da Consolação, 2000",
		City:        "São Paulo",
		State:       "SP",
		PostalCode:  "01302-001",
		Notes:       "Teste fonético Z/S terminal e acentuação (Luís -> Luiz)",
	},
	{
		DisplayName: "Phelipe Ramos",
		BirthDate:   "1988-04-12",
		Sex:         patientmodel.SexMale,
		Phone:       "11987654326",
		Email:       "phelipe.ramos@example.com",
		Street:      "Av Brigadeiro Faria Lima, 3000",
		City:        "São Paulo",
		State:       "SP",
		PostalCode:  "01451-000",
		Notes:       "Teste dígrafo PH/F (Felipe / Filipe -> Phelipe)",
	},
	{
		DisplayName: "Sandro Watanabe",
		BirthDate:   "1975-12-03",
		Sex:         patientmodel.SexMale,
		Phone:       "11987654327",
		Email:       "sandro.watanabe@example.com",
		Street:      "Rua Galvão Bueno, 200",
		City:        "São Paulo",
		State:       "SP",
		PostalCode:  "01506-000",
		Notes:       "Teste japonês / W inicial (Vatanabe -> Watanabe)",
	},
	{
		DisplayName: "Helena Kowalski",
		BirthDate:   "1992-06-25",
		Sex:         patientmodel.SexFemale,
		Phone:       "11987654328",
		Email:       "helena.kowalski@example.com",
		Street:      "Rua Domingos de Morais, 1200",
		City:        "São Paulo",
		State:       "SP",
		PostalCode:  "04010-100",
		Notes:       "Teste eslavo / polonês Beider-Morse (Kovalski -> Kowalski, Elena -> Helena)",
	},
	{
		DisplayName: "Conceição Aparecida",
		BirthDate:   "1968-08-15",
		Sex:         patientmodel.SexFemale,
		Phone:       "11987654329",
		Email:       "conceicao.aparecida@example.com",
		Street:      "Rua Vergueiro, 900",
		City:        "São Paulo",
		State:       "SP",
		PostalCode:  "01504-000",
		Notes:       "Teste acentuação e cedilha PT-BR (Conceicao -> Conceição)",
	},
	{
		DisplayName: "Roberto Müller",
		BirthDate:   "1980-02-28",
		Sex:         patientmodel.SexMale,
		Phone:       "11987654330",
		Email:       "roberto.muller@example.com",
		Street:      "Alameda Santos, 1100",
		City:        "São Paulo",
		State:       "SP",
		PostalCode:  "01419-002",
		Notes:       "Teste trema alemão BMPM (Muller, Mueller -> Müller)",
	},
	{
		DisplayName: "Arthur Zimmermann",
		BirthDate:   "1986-05-19",
		Sex:         patientmodel.SexMale,
		Phone:       "11987654331",
		Email:       "arthur.zimmermann@example.com",
		Street:      "Rua Bela Cintra, 750",
		City:        "São Paulo",
		State:       "SP",
		PostalCode:  "01415-000",
		Notes:       "Teste germânico TH e Z (Artur, Zimmerman, Simerman -> Arthur Zimmermann)",
	},
	{
		DisplayName: "Lucas Haddad",
		BirthDate:   "1989-11-21",
		Sex:         patientmodel.SexMale,
		Phone:       "11987654332",
		Email:       "lucas.haddad@example.com",
		Street:      "Rua Pamplona, 500",
		City:        "São Paulo",
		State:       "SP",
		PostalCode:  "01405-000",
		Notes:       "Teste árabe / consoante dupla (Hadad -> Haddad)",
	},
	{
		DisplayName: "Maria Joana da Silva",
		BirthDate:   "1994-04-02",
		Sex:         patientmodel.SexFemale,
		Phone:       "11987654333",
		Email:       "maria.joana@example.com",
		Street:      "Rua Voluntários da Pátria, 350",
		City:        "São Paulo",
		State:       "SP",
		PostalCode:  "02011-000",
		Notes:       "Nome comum composto brasileiro com partículas (da)",
	},
	{
		DisplayName: "Thiago Albuquerque",
		BirthDate:   "1993-08-10",
		Sex:         patientmodel.SexMale,
		Phone:       "11987654334",
		Email:       "thiago.albuquerque@example.com",
		Street:      "Rua Teodoro Sampaio, 1800",
		City:        "São Paulo",
		State:       "SP",
		PostalCode:  "05406-150",
		Notes:       "Teste TH e QU (Tiago -> Thiago, Albukerke -> Albuquerque)",
	},
	{
		DisplayName: "Yara Nogueira",
		BirthDate:   "1997-12-01",
		Sex:         patientmodel.SexFemale,
		Phone:       "11987654335",
		Email:       "yara.nogueira@example.com",
		Street:      "Rua Haddock Lobo, 600",
		City:        "São Paulo",
		State:       "SP",
		PostalCode:  "01414-000",
		Notes:       "Teste Y inicial (Iara -> Yara)",
	},
}

func main() {
	config.RegisterFlags(pflag.CommandLine)
	pflag.Parse()

	app := fx.New(
		config.Module,
		telemetry.Module,
		keystore.Module,
		meta.Module,
		crypto.Module,
		fx.WithLogger(telemetry.FxLogger),
		database.Module,
		storage.Module,
		audit.Module,
		auth.Module,
		clinicdomain.Module,
		policy.Module,
		userdomain.Module,
		identifier.Module,
		patientdomain.Module,
		fx.Provide(echo.New),
		fx.Invoke(registerSeedHook),
	)

	startCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := app.Start(startCtx); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting seed application: %v\n", err)
		os.Exit(1)
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	if err := app.Stop(stopCtx); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping seed application: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nSeed process completed successfully!")
}

func registerSeedHook(
	lc fx.Lifecycle,
	cfg *config.Config,
	client *record.Client,
	engine *crypto.Engine,
	userSvc *userusecase.Service,
	patientSvc *patientusecase.Service,
	logger log.Logger,
) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return runSeed(ctx, cfg, client, engine, userSvc, patientSvc, logger)
		},
	})
}

func runSeed(
	ctx context.Context,
	cfg *config.Config,
	client *record.Client,
	engine *crypto.Engine,
	userSvc *userusecase.Service,
	patientSvc *patientusecase.Service,
	logger log.Logger,
) error {
	fmt.Println("=== Seeding LibreVita Test Data ===")

	// 1. Get or create Clinic
	skipCtx := clinicctx.WithSkipIsolation(ctx)
	targetDomain := "demo.localhost"

	clinicRow, err := client.Clinic.Query().Where(clinic.DomainEQ(targetDomain)).First(skipCtx)
	if err != nil && !record.IsNotFound(err) {
		return errors.Wrap(err, "query clinic")
	}

	if clinicRow == nil {
		clinicID := ident.New[ident.ClinicID]()
		fmt.Printf("Creating clinic %q (domain: %s)...\n", "Clínica Modelo", targetDomain)
		clinicRow, err = client.Clinic.Create().
			SetID(clinicID).
			SetName("Clínica Modelo").
			SetDomain(targetDomain).
			SetCountry("BR").
			SetTimezone("America/Sao_Paulo").
			Save(skipCtx)
		if err != nil {
			return errors.Wrap(err, "create clinic")
		}
		fmt.Printf("Clinic created: %s (ID: %s)\n", clinicRow.Name, clinicRow.ID)
	} else {
		fmt.Printf("Found existing clinic %s (ID: %s)\n", clinicRow.Name, clinicRow.ID)
	}

	// 2. Set up clinic context and Ensure Clinic DEK
	clinicInfo := &clinicctx.Clinic{
		ID:          clinicRow.ID,
		Domain:      clinicRow.Domain,
		Name:        clinicRow.Name,
		Timezone:    clinicRow.Timezone,
		OnboardedAt: clinicRow.OnboardedAt,
	}
	clinicCtx := clinicctx.WithClinic(skipCtx, clinicInfo)
	clinicCtx = fle.WithClinicID(clinicCtx, clinicRow.ID)

	dek, err := engine.EnsureClinicDEK(clinicCtx, clinicRow.ID)
	if err != nil {
		return errors.Wrap(err, "ensure clinic DEK")
	}

	// 3. Onboard clinic if needed
	adminEmail := "admin@demo.localhost"
	adminPass := "Admin123!"
	adminName := "Administrador Modelo"

	if clinicRow.OnboardedAt == nil {
		fmt.Printf("Onboarding clinic with admin %s (%s)...\n", adminName, adminEmail)
		_, _, err := userSvc.Onboard(clinicCtx, userusecase.RegisterInput{
			Name:     adminName,
			Email:    adminEmail,
			Password: adminPass,
		}, nil)
		if err != nil && !errors.Is(err, userusecase.ErrAlreadyOnboarded) {
			return errors.Wrap(err, "onboard clinic")
		}
		// Refresh clinicRow
		clinicRow, err = client.Clinic.Get(skipCtx, clinicRow.ID)
		if err != nil {
			return errors.Wrap(err, "refresh clinic")
		}
		clinicInfo.OnboardedAt = clinicRow.OnboardedAt
		fmt.Println("Clinic onboarded successfully!")
	} else {
		fmt.Println("Clinic is already onboarded.")
	}

	// 4. Find admin user ID for CreatedBy
	adminUser, err := client.User.Query().
		Where(user.ClinicIDEQ(clinicRow.ID), user.EmailEQ(strings.ToLower(adminEmail))).
		First(skipCtx)
	var adminUserID string
	if err == nil && adminUser != nil {
		adminUserID = adminUser.ID.String()
	}

	// 5. Setup Patient context with FLE encryptor, hasher, resolver
	pCtx := crypto.WithRequestKeyCache(clinicCtx)
	defer crypto.ClearRequestKeyCache(pCtx)

	enc, err := crypto.NewClinicEncryptor(dek)
	if err != nil {
		return errors.Wrap(err, "new clinic encryptor")
	}
	alg := cfg.Crypto.HashAlgorithm
	if alg == "" {
		alg = crypto.DefaultHashAlgorithm
	}
	hasher, err := crypto.NewHasherFromDEK(dek, crypto.WithHashAlgorithm(alg))
	if err != nil {
		return errors.Wrap(err, "new clinic hasher")
	}

	pCtx = fle.WithEncryptor(pCtx, enc)
	pCtx = fle.WithHasher(pCtx, hasher)
	pCtx = fle.WithPatientEncryptorResolver(pCtx, engine)

	// Check existing patients count
	patientCount, err := patientSvc.Count(pCtx, clinicRow.ID.String())
	if err != nil {
		return errors.Wrap(err, "count patients")
	}
	fmt.Printf("Current patients count in clinic: %d\n", patientCount)

	if patientCount == 0 {
		fmt.Println("Seeding test patients...")
		for _, ptInput := range testPatients {
			created, err := patientSvc.Create(pCtx, clinicRow.ID.String(), adminUserID, ptInput)
			if err != nil {
				return errors.Wrapf(err, "create patient %q", ptInput.DisplayName)
			}
			fmt.Printf(" -> Created patient: %s (ID: %s)\n", created.DisplayName, created.ID)
		}
		fmt.Printf("Successfully seeded %d patients!\n", len(testPatients))
	} else {
		fmt.Printf("Patients already exist (%d). Skipping creation.\n", patientCount)
	}

	return nil
}
