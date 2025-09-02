package workerpool

import (
	"sync"
)

type WorkerPool struct {
	workers   []*worker
	taskQueue chan func()
	waitQueue chan *waitTask
	stop      chan struct{}
	stopOnce  sync.Once
	stopWait  chan struct{}
	wg        sync.WaitGroup
	waitWg    sync.WaitGroup
}

type worker struct {
	id   int
	pool *WorkerPool
	stop chan struct{}
}

type waitTask struct {
	task func()
	done chan struct{}
}

func NewWorkerPool(numberOfWorkers int) *WorkerPool {
	if numberOfWorkers <= 0 {
		numberOfWorkers = 1
	}

	wp := &WorkerPool{
		workers:   make([]*worker, numberOfWorkers),
		taskQueue: make(chan func(), 1000),
		waitQueue: make(chan *waitTask, 1000),
		stop:      make(chan struct{}),
		stopWait:  make(chan struct{}),
	}

	for i := 0; i < numberOfWorkers; i++ {
		w := &worker{
			id:   i,
			pool: wp,
			stop: make(chan struct{}),
		}
		wp.workers[i] = w
		wp.wg.Add(1)
		go w.run()
	}

	return wp
}

func (w *worker) run() {
	defer w.pool.wg.Done()

	for {
		select {
		case <-w.stop:
			return
		case <-w.pool.stop:
			return
		case <-w.pool.stopWait:
			return
		case task := <-w.pool.taskQueue:
			task()
		case waitTask := <-w.pool.waitQueue:
			waitTask.task()
			close(waitTask.done)
		}
	}
}

func (wp *WorkerPool) Submit(task func()) {
	select {
	case <-wp.stop:
		return
	case <-wp.stopWait:
		return
	default:
		wp.taskQueue <- task
	}
}

func (wp *WorkerPool) SubmitWait(task func()) {
	select {
	case <-wp.stop:
		task()
		return
	case <-wp.stopWait:
		task()
		return
	default:
		wt := &waitTask{
			task: task,
			done: make(chan struct{}),
		}
		wp.waitQueue <- wt
		<-wt.done
	}
}

func (wp *WorkerPool) Stop() {
	wp.stopOnce.Do(func() {
		close(wp.stop)
		for _, w := range wp.workers {
			close(w.stop)
		}
		wp.wg.Wait()
	})
}

func (wp *WorkerPool) StopWait() {
	wp.stopOnce.Do(func() {
		close(wp.stopWait)
		for _, w := range wp.workers {
			close(w.stop)
		}

		go wp.processRemainingTasks()

		wp.wg.Wait()
		wp.waitWg.Wait()
	})
}

func (wp *WorkerPool) processRemainingTasks() {
	wp.waitWg.Add(1)
	defer wp.waitWg.Done()

	for {
		select {
		case task := <-wp.taskQueue:
			task()
		default:
		}

		if len(wp.taskQueue) == 0 {
			break
		}
	}

	for {
		select {
		case waitTask := <-wp.waitQueue:
			waitTask.task()
			close(waitTask.done)
		default:
		}

		if len(wp.waitQueue) == 0 {
			break
		}
	}
}
