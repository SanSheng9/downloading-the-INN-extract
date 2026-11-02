package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mxschmitt/playwright-go"
)

const serviceURL = "https://service.nalog.ru/inn.do"

type PersonData struct {
	LastName       string `json:"lastName"`
	FirstName      string `json:"firstName"`
	MiddleName     string `json:"middleName"`
	BirthDate      string `json:"birthDate"`
	DocumentType   string `json:"documentType"`
	DocumentNumber string `json:"documentNumber"`
	DocumentDate   string `json:"documentDate"`
}

func main() {
	hideConsole()

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Проверяем аргументы командной строки.
	if len(os.Args) < 2 {
		return fmt.Errorf("укажи путь к JSON-файлу с данными")
	}

	// Загружаем данные физического лица.
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		return fmt.Errorf("не удалось прочитать JSON-файл: %w", err)
	}

	var person PersonData

	if err := json.Unmarshal(data, &person); err != nil {
		return fmt.Errorf("ошибка чтения JSON: %w", err)
	}

	fmt.Println("Данные успешно загружены.")

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("не удалось определить путь программы: %w", err)
	}

	baseDir := filepath.Dir(exePath)

	driverPath := filepath.Join(
		baseDir,
		"playwright",
		"driver",
	)

	browsersPath := filepath.Join(
		baseDir,
		"playwright",
		"browsers",
	)

	if err := os.Setenv("PLAYWRIGHT_DRIVER_PATH", driverPath); err != nil {
		return err
	}

	if err := os.Setenv("PLAYWRIGHT_BROWSERS_PATH", browsersPath); err != nil {
		return err
	}

	// Запускаем Playwright.
	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("не удалось запустить Playwright: %w", err)
	}
	defer pw.Stop()

	// Запускаем установленный Google Chrome.
	browser, err := pw.Chromium.Launch(
		playwright.BrowserTypeLaunchOptions{
			Headless: playwright.Bool(false),
			Channel:  playwright.String("chrome"),
		},
	)
	if err != nil {
		return fmt.Errorf("не удалось запустить Google Chrome: %w", err)
	}
	defer browser.Close()

	// Создаём страницу.
	context, err := browser.NewContext()
	if err != nil {
		return fmt.Errorf("не удалось создать контекст браузера: %w", err)
	}

	page, err := context.NewPage()
	if err != nil {
		return fmt.Errorf("не удалось создать страницу: %w", err)
	}

	page.OnDownload(handleDownload)

	page.OnDownload(func(download playwright.Download) {
		downloadPath := filepath.Join(
			os.Getenv("USERPROFILE"),
			"Downloads",
			"Выписки ИНН",
			download.SuggestedFilename(),
		)

		if err := os.MkdirAll(filepath.Dir(downloadPath), 0755); err != nil {
			log.Printf("Не удалось создать папку загрузок: %v", err)
			return
		}

		if err := download.SaveAs(downloadPath); err != nil {
			log.Printf("Не удалось сохранить файл: %v", err)
			return
		}

		fmt.Println("Файл сохранён:", downloadPath)
	})

	// Открываем сайт ФНС.
	_, err = page.Goto(serviceURL)
	if err != nil {
		return fmt.Errorf("не удалось открыть сайт ФНС: %w", err)
	}

	fmt.Println("Сайт ФНС открыт.")

	// Автоматически обрабатываем экран согласия.
	if err := acceptPersonalData(page); err != nil {
		return fmt.Errorf("ошибка подтверждения согласия: %w", err)
	}

	// Заполняем форму.
	if err := fillForm(page, person); err != nil {
		return fmt.Errorf("ошибка заполнения формы: %w", err)
	}

	fmt.Println("Форма заполнена.")
	fmt.Println("Проверь данные в браузере.")
	fmt.Println("Запрос в ФНС автоматически не отправляется.")

	// Ожидаем завершения работы пользователя.
	fmt.Println("Ожидаем закрытия браузера...")

	_, err = page.WaitForEvent("close")
	if err != nil {
		return fmt.Errorf("ошибка ожидания закрытия браузера: %w", err)
	}

	return nil

}

func formatDate(value string) (string, error) {
	if value == "" {
		return "", nil
	}

	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return "", fmt.Errorf("неверный формат даты %s: %w", value, err)
	}

	return date.Format("02.01.2006"), nil
}

func normalizeDocumentNumber(value string, documentType string) string {
	if documentType != "21" {
		return value
	}

	// Убираем пробелы и оставляем только цифры.
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)

	// Паспорт РФ: 99 99 9999990.
	if len(digits) == 10 || len(digits) == 11 {
		return digits[:2] + " " + digits[2:4] + " " + digits[4:]
	}

	return value
}

func fillForm(page playwright.Page, person PersonData) error {
	// Ждём появления основной формы.
	if err := page.Locator("#frmInn").WaitFor(); err != nil {
		return fmt.Errorf("форма ФНС не найдена: %w", err)
	}

	// Фамилия.
	if err := page.Locator("#fam").Fill(person.LastName); err != nil {
		return err
	}

	// Имя.
	if err := page.Locator("#nam").Fill(person.FirstName); err != nil {
		return err
	}

	// Отчество.
	if person.MiddleName != "" {
		if err := page.Locator("#otch").Fill(person.MiddleName); err != nil {
			return err
		}
	} else {
		// Используем видимый переключатель «Нет отчества».
		if err := page.Locator("#unichk_0").Click(); err != nil {
			return fmt.Errorf("не удалось установить отметку «Нет отчества»: %w", err)
		}
	}

	// Дата рождения.
	birthDate, err := formatDate(person.BirthDate)
	if err != nil {
		return err
	}

	if err := page.Locator("#bdate").Fill(birthDate); err != nil {
		return err
	}

	// Выбираем вид документа.
	// Используем JavaScript, поскольку стандартный select скрыт.
	_, err = page.Locator("#doctype").Evaluate(
		`(el, value) => {
			const option = Array.from(el.options).find(o => o.value === value);

			if (!option) {
				throw new Error("Не найден вид документа: " + value);
			}

			el.value = value;
			el.dispatchEvent(new Event("input", { bubbles: true }));
			el.dispatchEvent(new Event("change", { bubbles: true }));

			return el.value;
		}`,
		person.DocumentType,
	)

	if err != nil {
		return fmt.Errorf("ошибка выбора вида документа: %w", err)
	}

	// Серия и номер документа.
	documentNumber := normalizeDocumentNumber(
		person.DocumentNumber,
		person.DocumentType,
	)

	if err := page.Locator("#docno").Fill(documentNumber); err != nil {
		return fmt.Errorf("ошибка заполнения серии и номера документа: %w", err)
	}

	// Дата выдачи документа.
	if person.DocumentDate != "" {
		documentDate, err := formatDate(person.DocumentDate)
		if err != nil {
			return fmt.Errorf("ошибка формата даты выдачи: %w", err)
		}

		if err := page.Locator("#docdt").Fill(documentDate); err != nil {
			return fmt.Errorf("ошибка заполнения даты выдачи документа: %w", err)
		}
	}

	fmt.Println("Все поля успешно заполнены.")

	return nil
}

func acceptPersonalData(page playwright.Page) error {

	// Проверяем, не открылась ли уже основная форма.
	form := page.Locator("#frmInn")

	formVisible, err := form.IsVisible()
	if err != nil {
		return err
	}

	if formVisible {
		fmt.Println("Основная форма уже открыта.")
		return nil
	}

	// Находим видимый переключатель согласия.
	consent := page.Locator("#unichk_0")

	if err := consent.WaitFor(
		playwright.LocatorWaitForOptions{
			State: playwright.WaitForSelectorStateVisible,
		},
	); err != nil {
		return fmt.Errorf("не найден переключатель согласия: %w", err)
	}

	// Нажимаем на видимый переключатель.
	if err := consent.Click(); err != nil {
		return fmt.Errorf("не удалось нажать на согласие: %w", err)
	}

	// Проверяем состояние скрытого checkbox.
	checked, err := page.Locator("#personalData").Evaluate(
		`(el) => el.checked`,
		nil,
	)

	if err != nil {
		return fmt.Errorf("не удалось проверить согласие: %w", err)
	}

	if checked != true {
		return fmt.Errorf("флажок согласия не установлен")
	}

	fmt.Println("Согласие отмечено.")

	// Проверяем доступность кнопки.
	button := page.Locator("#btnContinue")

	enabled, err := button.IsEnabled()
	if err != nil {
		return fmt.Errorf("ошибка проверки кнопки продолжения: %w", err)
	}

	if !enabled {
		return fmt.Errorf("кнопка продолжения неактивна")
	}

	// Отправляем подтверждение согласия.
	if err := button.Click(); err != nil {
		return fmt.Errorf("не удалось нажать Продолжить: %w", err)
	}

	fmt.Println("Подтверждение отправлено. Ожидаем форму ФНС.")

	// Ждём перехода к основной форме.
	if err := form.WaitFor(
		playwright.LocatorWaitForOptions{
			State: playwright.WaitForSelectorStateVisible,
		},
	); err != nil {
		return fmt.Errorf("основная форма ФНС не появилась: %w", err)
	}

	fmt.Println("Основная форма ФНС открыта.")

	return nil
}

func handleDownload(download playwright.Download) {
	// Получаем каталог загрузок текущего пользователя.
	userProfile := os.Getenv("USERPROFILE")

	if userProfile == "" {
		log.Println("Не удалось определить профиль пользователя Windows")
		return
	}

	// Каталог для сохранения PDF.
	downloadDir := filepath.Join(
		userProfile,
		"Downloads",
		"Выписки ИНН",
	)

	// Создаём каталог, если его ещё нет.
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		log.Printf("Ошибка создания каталога: %v", err)
		return
	}

	// Имя скачиваемого файла.
	fileName := filepath.Base(download.SuggestedFilename())

	if fileName == "." || fileName == "" {
		log.Println("Не удалось определить имя скачиваемого файла")
		return
	}

	downloadPath := filepath.Join(downloadDir, fileName)

	// Сохраняем скачанный файл.
	if err := download.SaveAs(downloadPath); err != nil {
		log.Printf("Ошибка сохранения файла: %v", err)
		return
	}

	log.Println("Файл сохранён:", downloadPath)

	// Открываем файл стандартной программой Windows.
	cmd := exec.Command(
		"cmd",
		"/c",
		"start",
		"",
		downloadPath,
	)

	if err := cmd.Start(); err != nil {
		log.Printf("Не удалось открыть PDF: %v", err)
		return
	}

	log.Println("PDF открыт.")
}
