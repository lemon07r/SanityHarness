#[derive(Copy, Clone, Debug, PartialEq, Eq, Hash)]
pub struct Handle {
    index: usize,
    generation: u32,
}

impl Handle {
    pub fn index(self) -> usize {
        self.index
    }

    pub fn generation(self) -> u32 {
        self.generation
    }
}

pub struct Arena<T> {
    // TODO: Add fields.
    _marker: std::marker::PhantomData<T>,
}

impl<T> Arena<T> {
    pub fn new() -> Self {
        todo!("Implement Arena::new")
    }

    pub fn len(&self) -> usize {
        todo!("Implement Arena::len")
    }

    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }

    pub fn insert(&mut self, value: T) -> Handle {
        let _ = value;
        todo!("Implement Arena::insert")
    }

    pub fn get(&self, handle: Handle) -> Option<&T> {
        let _ = handle;
        todo!("Implement Arena::get")
    }

    pub fn get_mut(&mut self, handle: Handle) -> Option<&mut T> {
        let _ = handle;
        todo!("Implement Arena::get_mut")
    }

    pub fn remove(&mut self, handle: Handle) -> Option<T> {
        let _ = handle;
        todo!("Implement Arena::remove")
    }
}
