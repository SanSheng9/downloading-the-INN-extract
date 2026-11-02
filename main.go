package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
	"strings"

	"github.com/mxschmitt/playwright-go"
)

const serviceURL = "https://service.nalog.ru/inn.do"

type PersonData struct {
	LastName      string `json:"lastName"`
	FirstName     string `json:"firstName"`
	MiddleName    string `json:"middleName"`
	BirthDate     string `json:"birthDate"`
	DocumentType  string `json:"documentType"`
	DocumentNumber string `json:"documentNumber"`
	DocumentDate  string `json:"documentDate"`
}

func main() {
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
    page, err := browser.NewPage()
    if err != nil {
        return fmt.Errorf("не удалось создать страницу браузера: %w", err)
    }

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
    fmt.Println("Нажми Enter для закрытия браузера.")

    // Ожидаем завершения работы пользователя.
    fmt.Scanln()

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