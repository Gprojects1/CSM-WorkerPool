package workerpool

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPool_Submit(t *testing.T) {
	wp := NewWorkerPool(2)
	defer wp.Stop()

	var counter int32
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		wp.Submit(func() {
			defer wg.Done()
			atomic.AddInt32(&counter, 1)
		})
	}

	wg.Wait()

	if atomic.LoadInt32(&counter) != 10 {
		t.Errorf("Expected 10 tasks executed, got %d", atomic.LoadInt32(&counter))
	}
}

func TestWorkerPool_SubmitWait(t *testing.T) {
	wp := NewWorkerPool(2)
	defer wp.Stop()

	var counter int32

	for i := 0; i < 5; i++ {
		wp.SubmitWait(func() {
			atomic.AddInt32(&counter, 1)
		})
	}

	if atomic.LoadInt32(&counter) != 5 {
		t.Errorf("Expected 5 tasks executed, got %d", atomic.LoadInt32(&counter))
	}
}

func TestWorkerPool_Stop(t *testing.T) {
	wp := NewWorkerPool(2)

	// Добавляем задачу, которая выполняется долго
	var taskStarted bool
	var taskCompleted bool

	wp.Submit(func() {
		taskStarted = true
		time.Sleep(100 * time.Millisecond)
		taskCompleted = true
	})

	// Даем задаче начать выполнение
	time.Sleep(10 * time.Millisecond)

	// Останавливаем пул - должен дождаться только текущей задачи
	wp.Stop()

	if !taskStarted {
		t.Error("Task should have started")
	}
	if !taskCompleted {
		t.Error("Stop should wait for current task to complete")
	}
}

func TestWorkerPool_StopWait(t *testing.T) {
	wp := NewWorkerPool(2)

	var completedTasks int32
	var completionOrder sync.Map
	var wg sync.WaitGroup

	// Добавляем несколько задач с задержкой
	for i := 0; i < 5; i++ {
		taskID := i
		wg.Add(1)
		wp.Submit(func() {
			defer wg.Done()
			defer atomic.AddInt32(&completedTasks, 1)
			// Небольшая задержка для имитации работы
			time.Sleep(5 * time.Millisecond)
			completionOrder.Store(taskID, true)
		})
	}

	// Даем задачам время попасть в очередь
	time.Sleep(1 * time.Millisecond)

	// Останавливаем с ожиданием всех задач
	wp.StopWait()

	// Проверяем, что все задачи выполнены
	if atomic.LoadInt32(&completedTasks) != 5 {
		t.Errorf("Expected 5 tasks completed, got %d", atomic.LoadInt32(&completedTasks))
	}

	// Проверяем, что все задачи действительно выполнены
	for i := 0; i < 5; i++ {
		if _, ok := completionOrder.Load(i); !ok {
			t.Errorf("Task %d was not completed", i)
		}
	}
}

func TestWorkerPool_SubmitAfterStop(t *testing.T) {
	wp := NewWorkerPool(2)
	wp.Stop()

	var taskExecuted bool
	wp.Submit(func() {
		taskExecuted = true
	})

	time.Sleep(10 * time.Millisecond)

	if taskExecuted {
		t.Error("Task should not be executed after stop")
	}
}

func TestWorkerPool_SubmitWaitAfterStop(t *testing.T) {
	wp := NewWorkerPool(2)
	wp.Stop()

	var taskExecuted bool
	wp.SubmitWait(func() {
		taskExecuted = true
	})

	if !taskExecuted {
		t.Error("SubmitWait should execute task synchronously after stop")
	}
}

func TestWorkerPool_ConcurrentUsage(t *testing.T) {
	wp := NewWorkerPool(4)
	defer wp.StopWait()

	var counter int32
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wp.Submit(func() {
				atomic.AddInt32(&counter, 1)
			})
		}()
	}

	wg.Wait()
	wp.StopWait()

	if atomic.LoadInt32(&counter) != 100 {
		t.Errorf("Expected 100 tasks executed, got %d", atomic.LoadInt32(&counter))
	}
}

func TestWorkerPool_ZeroWorkers(t *testing.T) {
	wp := NewWorkerPool(0) // Должно использоваться значение по умолчанию (1)
	defer wp.Stop()

	var taskExecuted bool
	wp.Submit(func() {
		taskExecuted = true
	})

	time.Sleep(10 * time.Millisecond)

	if !taskExecuted {
		t.Error("Task should be executed even with zero workers")
	}
}

func TestWorkerPool_MultipleStopCalls(t *testing.T) {
	wp := NewWorkerPool(2)

	// Multiple stop calls should not panic
	wp.Stop()
	wp.Stop()
	wp.StopWait()
}

func TestWorkerPool_StopWaitWithPendingTasks(t *testing.T) {
	wp := NewWorkerPool(1) // Один воркер для контроля

	// Блокируем воркер
	blocker := make(chan struct{})
	wp.Submit(func() {
		<-blocker
	})

	// Добавляем задачи в очередь
	for i := 0; i < 3; i++ {
		wp.Submit(func() {})
	}

	// Запускаем остановку с ожиданием в отдельной горутине
	stopDone := make(chan struct{})
	go func() {
		wp.StopWait()
		close(stopDone)
	}()

	// Даем время для начала остановки
	time.Sleep(10 * time.Millisecond)

	// Разблокируем воркер
	close(blocker)

	// Ждем завершения остановки
	select {
	case <-stopDone:
		// Успешно завершено
	case <-time.After(100 * time.Millisecond):
		t.Error("StopWait timed out")
	}
}
